package ui

import (
	"fmt"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/imkk000/htopx/internal/gpu"
	"github.com/imkk000/htopx/internal/sysinfo"
)

type tab int

const (
	tabMain tab = iota
	tabGPU
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeSearch
	modeKill
	modeHelp
)

type sampleMsg struct {
	sys  sysinfo.Snapshot
	gpus gpu.Snapshot
	err  error
}

type tickMsg struct{}

type Options struct {
	Delay       time.Duration
	ShowThreads bool
	ShowKernel  bool
	Tree        bool
}

type Model struct {
	sampler *sysinfo.Sampler
	monitor *gpu.Monitor
	delay   time.Duration
	samples int

	sys  sysinfo.Snapshot
	gpus gpu.Snapshot
	rows []row

	width, height int

	tab         tab
	mode        mode
	sortBy      sortKey
	sortDesc    bool
	tree        bool
	showThreads bool
	showKernel  bool
	filter      string
	search      string

	selPID   int
	follow   bool
	sel      int
	scroll   int
	status   string
	statusAt time.Time
}

func New(sampler *sysinfo.Sampler, monitor *gpu.Monitor, opts Options) *Model {
	return &Model{
		sampler:     sampler,
		monitor:     monitor,
		delay:       opts.Delay,
		sortBy:      sortCPU,
		sortDesc:    true,
		tree:        opts.Tree,
		showThreads: opts.ShowThreads,
		showKernel:  opts.ShowKernel,
		width:       80,
		height:      24,
	}
}

func (m *Model) Init() tea.Cmd {
	return m.sample()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
	case sampleMsg:
		if msg.err != nil {
			m.setStatus("sample: " + msg.err.Error())
		} else {
			m.sys, m.gpus = msg.sys, msg.gpus
		}
		m.samples++
		if m.status != "" && time.Since(m.statusAt) > 4*time.Second {
			m.status = ""
		}
		m.refresh()
		return m, m.tick()
	case tickMsg:
		return m, m.sample()
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-3)
		case tea.MouseWheelDown:
			m.move(3)
		}
	case tea.MouseClickMsg:
		m.handleClick(msg.X, msg.Y)
	}
	return m, nil
}

func (m *Model) sample() tea.Cmd {
	sampler, monitor := m.sampler, m.monitor
	return func() tea.Msg {
		sys, err := sampler.Sample()
		return sampleMsg{sys: sys, gpus: monitor.Sample(), err: err}
	}
}

func (m *Model) tick() tea.Cmd {
	delay := m.delay
	if m.samples == 1 {
		delay = 250 * time.Millisecond
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) setStatus(s string) {
	m.status, m.statusAt = s, time.Now()
}

func (m *Model) refresh() {
	m.rows = m.buildRows()
	if m.follow {
		if i := slices.IndexFunc(m.rows, func(r row) bool { return r.PID == m.selPID }); i >= 0 {
			m.sel = i
		} else {
			m.follow = false
		}
	}
	m.clamp()
}

func (m *Model) clamp() {
	m.sel = max(min(m.sel, len(m.rows)-1), 0)
	if len(m.rows) > 0 {
		m.selPID = m.rows[m.sel].PID
	}
	page := m.pageSize()
	if m.sel < m.scroll {
		m.scroll = m.sel
	}
	if m.sel >= m.scroll+page {
		m.scroll = m.sel - page + 1
	}
	m.scroll = max(min(m.scroll, len(m.rows)-page), 0)
}

func (m *Model) move(delta int) {
	m.follow = false
	m.sel += delta
	m.clamp()
}

func (m *Model) selected() (row, bool) {
	if m.sel < 0 || m.sel >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.sel], true
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	switch m.mode {
	case modeFilter, modeSearch:
		m.handleInput(key, k.Text)
		return nil
	case modeKill:
		m.handleKill(key)
		return nil
	case modeHelp:
		m.mode = modeNormal
		return nil
	}

	if key == "q" || key == "f10" {
		return tea.Quit
	}
	if action, ok := keyActions[key]; ok {
		action(m)
	}
	return nil
}

func (m *Model) switchTab(t tab) {
	m.tab = t
	if t == tabGPU && m.gpus.Kind == gpu.KindNone {
		return
	}
	if !slices.ContainsFunc(m.columnsFor(t), func(c column) bool { return c.key == m.sortBy }) {
		m.sortBy, m.sortDesc = sortCPU, true
		if t == tabGPU {
			m.sortBy = sortGPU
		}
	}
	m.refresh()
}

func (m *Model) handleInput(key, text string) {
	searching := m.mode == modeSearch
	target := &m.filter
	if searching {
		target = &m.search
	}
	switch key {
	case "esc":
		*target = ""
		m.mode = modeNormal
	case "enter":
		m.mode = modeNormal
	case "backspace":
		if r := []rune(*target); len(r) > 0 {
			*target = string(r[:len(r)-1])
		}
	case "f3":
		if searching {
			m.findNext(m.search, m.sel+1)
		}
		return
	default:
		if text == "" {
			return
		}
		*target += text
	}
	if searching {
		m.findNext(m.search, m.sel)
		return
	}
	m.refresh()
}

func (m *Model) findNext(needle string, from int) {
	if needle == "" || len(m.rows) == 0 {
		return
	}
	needle = strings.ToLower(needle)
	for i := range len(m.rows) {
		idx := (from + i) % len(m.rows)
		if strings.Contains(strings.ToLower(m.rows[idx].Command), needle) {
			m.sel = idx
			m.clamp()
			return
		}
	}
}

func (m *Model) setSort(k sortKey) {
	if m.sortBy == k {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortBy, m.sortDesc = k, defaultDesc(k)
	}
	m.refresh()
}

func (m *Model) cycleSort(dir int) {
	keys := make([]sortKey, 0, 16)
	for _, c := range m.columns() {
		keys = append(keys, c.key)
	}
	keys = append(keys, sortCommand)
	i := slices.Index(keys, m.sortBy)
	i = (i + dir + len(keys)) % len(keys)
	m.sortBy, m.sortDesc = keys[i], defaultDesc(keys[i])
	m.refresh()
}

func (m *Model) renice(delta int) {
	r, ok := m.selected()
	if !ok {
		return
	}
	nice := max(min(r.Nice+delta, 19), -20)
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, r.PID, nice); err != nil {
		m.setStatus(fmt.Sprintf("renice %d: %v", r.PID, err))
		return
	}
	r.Nice = nice
	m.setStatus(fmt.Sprintf("PID %d nice set to %d", r.PID, nice))
}

var killSignals = map[string]syscall.Signal{
	"enter": syscall.SIGTERM,
	"t":     syscall.SIGTERM,
	"9":     syscall.SIGKILL,
	"1":     syscall.SIGHUP,
	"2":     syscall.SIGINT,
	"s":     syscall.SIGSTOP,
	"c":     syscall.SIGCONT,
}

func (m *Model) handleKill(key string) {
	m.mode = modeNormal
	sig, ok := killSignals[key]
	if !ok {
		return
	}
	r, ok := m.selected()
	if !ok {
		return
	}
	if err := syscall.Kill(r.PID, sig); err != nil {
		m.setStatus(fmt.Sprintf("kill %d: %v", r.PID, err))
		return
	}
	m.setStatus(fmt.Sprintf("sent %s to PID %d", sig, r.PID))
}

func (m *Model) handleClick(x, y int) {
	top := m.tableTop()
	switch {
	case y == top-1:
		cols := m.columns()
		pos := 0
		for _, c := range cols {
			if x < pos+c.width+1 {
				m.setSort(c.key)
				return
			}
			pos += c.width + 1
		}
		m.setSort(sortCommand)
	case y >= top && y < top+m.pageSize():
		m.sel = m.scroll + y - top
		m.clamp()
		m.follow = true
	}
}

func toggle(get func(m *Model) *bool) func(m *Model) {
	return func(m *Model) {
		v := get(m)
		*v = !*v
		m.refresh()
	}
}

func sortAction(k sortKey) func(m *Model) {
	return func(m *Model) { m.setSort(k) }
}

func bindKeys(actions map[string]func(m *Model), action func(m *Model), keys ...string) {
	for _, k := range keys {
		actions[k] = action
	}
}

var keyActions = func() map[string]func(m *Model) {
	a := make(map[string]func(m *Model))
	bindKeys(a, func(m *Model) { m.move(-1) }, "up")
	bindKeys(a, func(m *Model) { m.move(1) }, "down")
	bindKeys(a, func(m *Model) { m.move(-m.pageSize()) }, "pgup", "ctrl+b")
	bindKeys(a, func(m *Model) { m.move(m.pageSize()) }, "pgdown", "ctrl+f", "space")
	bindKeys(a, func(m *Model) { m.move(-len(m.rows)) }, "home", "g")
	bindKeys(a, func(m *Model) { m.move(len(m.rows)) }, "end", "G")
	bindKeys(a, func(m *Model) { m.switchTab((m.tab + 1) % 2) }, "tab", "shift+tab")
	bindKeys(a, func(m *Model) { m.mode = modeHelp }, "f1", "h", "?")
	bindKeys(a, func(m *Model) { m.mode, m.search = modeSearch, "" }, "f3", "/")
	bindKeys(a, func(m *Model) { m.findNext(m.search, m.sel+1) }, "n")
	bindKeys(a, func(m *Model) { m.mode = modeFilter }, "f4", "\\")
	bindKeys(a, toggle(func(m *Model) *bool { return &m.tree }), "f5", "t")
	bindKeys(a, toggle(func(m *Model) *bool { return &m.sortDesc }), "I")
	bindKeys(a, toggle(func(m *Model) *bool { return &m.showThreads }), "H")
	bindKeys(a, toggle(func(m *Model) *bool { return &m.showKernel }), "K")
	bindKeys(a, func(m *Model) { m.cycleSort(1) }, "f6", ">", ".")
	bindKeys(a, func(m *Model) { m.cycleSort(-1) }, "<", ",")
	bindKeys(a, func(m *Model) { m.follow = true }, "F")
	bindKeys(a, sortAction(sortCPU), "P")
	bindKeys(a, sortAction(sortMem), "M")
	bindKeys(a, sortAction(sortTime), "T")
	bindKeys(a, sortAction(sortPID), "N")
	bindKeys(a, sortAction(sortGPU), "U")
	bindKeys(a, sortAction(sortGMem), "V")
	bindKeys(a, func(m *Model) { m.renice(-1) }, "f7", "]")
	bindKeys(a, func(m *Model) { m.renice(1) }, "f8", "[")
	bindKeys(a, func(m *Model) {
		if _, ok := m.selected(); ok {
			m.mode = modeKill
		}
	}, "f9", "k")
	return a
}()
