package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/imkk000/htopx/internal/gpu"
)

var helpLines = []string{
	"htopx - htop with NVIDIA GPU monitoring",
	"",
	"CPU bar:  [low-priority/normal/kernel/virt          used%]",
	"Mem bar:  [used/shared/buffers/cache            used/total]",
	"Swp bar:  [used/cache                           used/total]",
	"GPU bars: GPU utilization, VRM memory, PWR power draw/cap",
	"Sensors:  CPU package temp, CPU fan speed, RAM temp (hottest DIMM)",
	"",
	"   Arrows PgUp PgDn Home End: move selection    Mouse: click to select, header to sort",
	"   Tab: switch Main / GPU process view",
	"   F3 /: search    n: next match              F4 \\: filter by command",
	"   F5 t: tree view                             F6 > .: next sort column   < ,: previous",
	"   P: sort CPU%   M: sort MEM%   T: sort TIME   N: sort PID   U: sort GPU%   V: sort GMEM",
	"   I: invert sort order                       F: follow selected process",
	"   H: toggle user threads                      K: toggle kernel threads",
	"   F7 ]: nice - (higher priority)              F8 [: nice + (lower priority)",
	"   F9 k: send signal                           F10 q: quit",
	"",
	"GPU view columns: GPU index, TYPE (C compute, G graphics), GPU% (SM), MBW% (memory bandwidth),",
	"ENC%/DEC% (video engines), GMEM (VRAM used by process)",
	"",
	"Press any key to return.",
}

func (m *Model) headerHeight() int {
	n := len(m.sys.Cores)
	cols := 1
	switch {
	case m.width >= 120:
		cols = 4
	case m.width >= 60:
		cols = 2
	}
	var rows int
	if n > 0 {
		cols = min(cols, n)
		rows = (n + cols - 1) / cols
	}
	height := rows + 3 + 3*len(m.gpus.Devices)
	if m.gpus.Driver != "" {
		height++
	}
	return height
}

func (m *Model) tableTop() int {
	return m.headerHeight() + 3
}

func (m *Model) pageSize() int {
	return max(m.height-m.tableTop()-1, 1)
}

func (m *Model) View() tea.View {
	lines := m.headerLines()
	lines = append(lines, "", m.tabBar(), m.columnHeader())
	if m.mode == modeHelp {
		lines = append(lines, m.helpScreen()...)
	} else {
		lines = append(lines, m.tableLines()...)
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:max(m.height-1, 0)], m.footer())
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "htopx"
	return v
}

func (m *Model) tabBar() string {
	t := func(name string, active bool) string {
		bg, fg := "44", cBlue
		if active {
			bg, fg = "42", cGreen
		}
		return paint(fg+";"+bg, "[") + paint(cBlack+";"+bg, name) + paint(fg+";"+bg, "]")
	}
	gpuName := "GPU"
	if n := len(m.gpus.Procs); n > 0 {
		gpuName += " " + strconv.Itoa(n)
	}
	bar := "  " + t("Main", m.tab == tabMain)
	if m.gpus.Kind == gpu.KindNone {
		return bar
	}
	return bar + " " + t(gpuName, m.tab == tabGPU)
}

func (m *Model) arrow() string {
	if m.sortDesc {
		return "▽"
	}
	return "△"
}

func (m *Model) columnHeader() string {
	cols := m.columns()
	parts := make([]string, 0, len(cols)+1)
	var used int
	for _, c := range cols {
		title := padRight(c.title, c.width)
		if c.right {
			title = padLeft(c.title, c.width)
		}
		used += c.width + 1
		if c.key == m.sortBy {
			parts = append(parts, paint(cOnCyan, title+m.arrow()))
			continue
		}
		parts = append(parts, paint(cOnGreen, title+" "))
	}
	rest := max(m.width-used, 0)
	if m.sortBy == sortCommand {
		parts = append(parts, paint(cOnCyan, fit("Command"+m.arrow(), rest)))
	} else {
		parts = append(parts, paint(cOnGreen, fit("Command", rest)))
	}
	return strings.Join(parts, "")
}

func (m *Model) tableLines() []string {
	page := m.pageSize()
	end := min(m.scroll+page, len(m.rows))
	out := make([]string, 0, page)
	cols := m.columns()
	for i := m.scroll; i < end; i++ {
		r := m.rows[i]
		p := painter(paint)
		if i == m.sel {
			p = plain
		}
		parts := make([]string, 0, len(cols)+1)
		var used int
		for _, c := range cols {
			cell := c.cell(r, p)
			if c.right {
				cell = padLeft(cell, c.width)
			} else {
				cell = fit(cell, c.width)
			}
			parts = append(parts, cell+" ")
			used += c.width + 1
		}
		cmd := ansi.Truncate(r.prefix+r.Command, max(m.width-used, 0), "")
		switch {
		case r.Thread:
			cmd = p(cGreen, cmd)
		case r.Kernel:
			cmd = p(cDim, cmd)
		}
		line := strings.Join(append(parts, cmd), "")
		if i == m.sel {
			line = paint(cOnCyan, fit(line, m.width))
		}
		out = append(out, line)
	}
	if len(m.rows) == 0 && m.tab == tabGPU {
		out = append(out, paint(cDim, "  no processes using the GPU"))
	}
	return out
}

func (m *Model) helpScreen() []string {
	out := make([]string, 0, len(helpLines))
	for _, l := range helpLines {
		out = append(out, " "+ansi.Truncate(l, m.width-1, ""))
	}
	return out
}

func fkey(key, label string) string {
	return key + paint(cOnCyan, label)
}

func (m *Model) footer() string {
	var s string
	switch m.mode {
	case modeFilter:
		s = fkey("Enter", "Done  ") + fkey("Esc", "Clear ") + " " + paint(cBold, "Filter: ") + m.filter
	case modeSearch:
		s = fkey("F3", "Next  ") + fkey("Enter", "Done  ") + fkey("Esc", "Cancel") + " " + paint(cBold, "Search: ") + m.search
	case modeKill:
		r, _ := m.selected()
		s = paint(cBold, "Signal PID "+strconv.Itoa(r.PID)+": ") + fkey("Enter", "TERM ") + fkey("9", "KILL ") +
			fkey("1", "HUP ") + fkey("2", "INT ") + fkey("s", "STOP ") + fkey("c", "CONT ") + fkey("Esc", "Cancel")
	default:
		s = fkey("F1", "Help  ") + fkey("F3", "Search") + fkey("F4", "Filter") + fkey("F5", "Tree  ") +
			fkey("F6", "SortBy") + fkey("F7", "Nice -") + fkey("F8", "Nice +") + fkey("F9", "Kill  ") +
			fkey("F10", "Quit  ")
		if m.gpus.Kind != gpu.KindNone {
			s += fkey("Tab", m.otherTab())
		}
		if m.filter != "" {
			s += " " + paint(cBold, "Filter: ") + paint(cYellow, m.filter)
		}
		if m.status != "" {
			s += " " + paint(cYellow, m.status)
		}
	}
	return ansi.Truncate(s, m.width, "")
}

func (m *Model) otherTab() string {
	if m.tab == tabGPU {
		return "Main  "
	}
	return "GPU   "
}
