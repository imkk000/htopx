package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/imkk000/htopx/internal/gpu"
	"github.com/imkk000/htopx/internal/sysinfo"
)

type sortKey int

const (
	sortPID sortKey = iota
	sortUser
	sortPri
	sortNice
	sortVirt
	sortRes
	sortShr
	sortState
	sortCPU
	sortMem
	sortGPU
	sortGMem
	sortGPUIdx
	sortGPUType
	sortGPUMemBW
	sortEnc
	sortDec
	sortGPUTime
	sortTime
	sortCommand
)

type row struct {
	*sysinfo.Process
	gpu    *gpu.ProcUsage
	prefix string
}

type column struct {
	title string
	width int
	right bool
	key   sortKey
	desc  bool
	cell  func(r row, p painter) string
}

func gpuNum(r row, get func(u *gpu.ProcUsage) uint32) string {
	if r.gpu == nil {
		return ""
	}
	return strconv.FormatUint(uint64(get(r.gpu)), 10)
}

var (
	colPID = column{"PID", 7, true, sortPID, false, func(r row, _ painter) string {
		return strconv.Itoa(r.PID)
	}}
	colUser = column{"USER", 10, false, sortUser, false, func(r row, _ painter) string {
		return r.User
	}}
	colPri = column{"PRI", 3, true, sortPri, false, func(r row, _ painter) string {
		if r.Priority <= -100 {
			return "RT"
		}
		return strconv.Itoa(r.Priority)
	}}
	colNice = column{"NI", 3, true, sortNice, false, func(r row, p painter) string {
		s := padLeft(strconv.Itoa(r.Nice), 3)
		switch {
		case r.Nice < 0:
			return p(cRed, s)
		case r.Nice > 0:
			return p(cBlue, s)
		}
		return p(cDim, s)
	}}
	colVirt = column{"VIRT", 5, true, sortVirt, true, func(r row, p painter) string {
		return kbytesCell(r.Virt, p)
	}}
	colRes = column{"RES", 5, true, sortRes, true, func(r row, p painter) string {
		return kbytesCell(r.Res, p)
	}}
	colShr = column{"SHR", 5, true, sortShr, true, func(r row, p painter) string {
		return kbytesCell(r.Shr, p)
	}}
	colState = column{"S", 1, false, sortState, false, func(r row, p painter) string {
		s := string(r.State)
		switch r.State {
		case 'R':
			return p(cBoldGreen, s)
		case 'D':
			return p(cBoldRed, s)
		case 'S', 'I':
			return p(cDim, s)
		}
		return s
	}}
	colCPU = column{"CPU%", 5, true, sortCPU, true, func(r row, _ painter) string {
		if r.CPU >= 999.95 {
			return fmt.Sprintf("%.0f", r.CPU)
		}
		return fmt.Sprintf("%.1f", r.CPU)
	}}
	colMem = column{"MEM%", 4, true, sortMem, true, func(r row, _ painter) string {
		return fmt.Sprintf("%.1f", r.Mem)
	}}
	colGPU = column{"GPU%", 4, true, sortGPU, true, func(r row, p painter) string {
		return p(cMagenta, gpuNum(r, func(u *gpu.ProcUsage) uint32 { return u.SM }))
	}}
	colGMem = column{"GMEM", 5, true, sortGMem, true, func(r row, p painter) string {
		if r.gpu == nil {
			return ""
		}
		return kbytesCell(r.gpu.MemUsed/1024, p)
	}}
	colGPUIdx = column{"GPU", 3, true, sortGPUIdx, false, func(r row, _ painter) string {
		if r.gpu == nil {
			return ""
		}
		return strconv.Itoa(r.gpu.GPU)
	}}
	colGPUType = column{"TYPE", 4, false, sortGPUType, false, func(r row, p painter) string {
		if r.gpu == nil {
			return ""
		}
		return p(cYellow, r.gpu.Type)
	}}
	colGPUMemBW = column{"MBW%", 4, true, sortGPUMemBW, true, func(r row, _ painter) string {
		return gpuNum(r, func(u *gpu.ProcUsage) uint32 { return u.Mem })
	}}
	colEnc = column{"ENC%", 4, true, sortEnc, true, func(r row, _ painter) string {
		return gpuNum(r, func(u *gpu.ProcUsage) uint32 { return u.Enc })
	}}
	colDec = column{"DEC%", 4, true, sortDec, true, func(r row, _ painter) string {
		return gpuNum(r, func(u *gpu.ProcUsage) uint32 { return u.Dec })
	}}
	colGPUTime = column{"GTIME", 9, true, sortGPUTime, true, func(r row, _ painter) string {
		if r.gpu == nil {
			return ""
		}
		return timeCell(uint64(r.gpu.GPUTime / (time.Second / sysinfo.ClockTicks)))
	}}
	colTime = column{"TIME+", 9, true, sortTime, true, func(r row, _ painter) string {
		return timeCell(r.Ticks)
	}}
)

var (
	baseColumns       = []column{colPID, colUser, colPri, colNice, colVirt, colRes, colShr, colState, colCPU, colMem, colTime}
	nvidiaMainColumns = []column{colPID, colUser, colPri, colNice, colVirt, colRes, colShr, colState, colCPU, colMem, colGPU, colGMem, colTime}
	appleMainColumns  = []column{colPID, colUser, colPri, colNice, colVirt, colRes, colShr, colState, colCPU, colMem, colGPU, colTime}
	nvidiaGPUColumns  = []column{colPID, colUser, colGPUIdx, colGPUType, colGPU, colGPUMemBW, colEnc, colDec, colGMem, colCPU, colMem, colTime}
	appleGPUColumns   = []column{colPID, colUser, colGPU, colGPUTime, colCPU, colMem, colTime}
)

func (m *Model) columnsFor(t tab) []column {
	switch m.gpus.Kind {
	case gpu.KindNVIDIA:
		if t == tabGPU {
			return nvidiaGPUColumns
		}
		return nvidiaMainColumns
	case gpu.KindApple:
		if t == tabGPU {
			return appleGPUColumns
		}
		return appleMainColumns
	}
	return baseColumns
}

func (m *Model) columns() []column {
	return m.columnsFor(m.tab)
}

func defaultDesc(k sortKey) bool {
	for _, set := range [][]column{nvidiaMainColumns, nvidiaGPUColumns, appleGPUColumns} {
		for _, c := range set {
			if c.key == k {
				return c.desc
			}
		}
	}
	return false
}

func byGPU[T cmp.Ordered](get func(u *gpu.ProcUsage) T) func(a, b row) int {
	value := func(r row) T {
		var zero T
		if r.gpu == nil {
			return zero
		}
		return get(r.gpu)
	}
	return func(a, b row) int { return cmp.Compare(value(a), value(b)) }
}

func byProc[T cmp.Ordered](get func(p *sysinfo.Process) T) func(a, b row) int {
	return func(a, b row) int { return cmp.Compare(get(a.Process), get(b.Process)) }
}

var comparators = map[sortKey]func(a, b row) int{
	sortPID:      byProc(func(p *sysinfo.Process) int { return p.PID }),
	sortUser:     byProc(func(p *sysinfo.Process) string { return p.User }),
	sortPri:      byProc(func(p *sysinfo.Process) int { return p.Priority }),
	sortNice:     byProc(func(p *sysinfo.Process) int { return p.Nice }),
	sortVirt:     byProc(func(p *sysinfo.Process) uint64 { return p.Virt }),
	sortRes:      byProc(func(p *sysinfo.Process) uint64 { return p.Res }),
	sortShr:      byProc(func(p *sysinfo.Process) uint64 { return p.Shr }),
	sortState:    byProc(func(p *sysinfo.Process) byte { return p.State }),
	sortCPU:      byProc(func(p *sysinfo.Process) float64 { return p.CPU }),
	sortMem:      byProc(func(p *sysinfo.Process) float64 { return p.Mem }),
	sortTime:     byProc(func(p *sysinfo.Process) uint64 { return p.Ticks }),
	sortCommand:  byProc(func(p *sysinfo.Process) string { return p.Command }),
	sortGPU:      byGPU(func(u *gpu.ProcUsage) uint32 { return u.SM }),
	sortGMem:     byGPU(func(u *gpu.ProcUsage) uint64 { return u.MemUsed }),
	sortGPUIdx:   byGPU(func(u *gpu.ProcUsage) int { return u.GPU }),
	sortGPUType:  byGPU(func(u *gpu.ProcUsage) string { return u.Type }),
	sortGPUMemBW: byGPU(func(u *gpu.ProcUsage) uint32 { return u.Mem }),
	sortEnc:      byGPU(func(u *gpu.ProcUsage) uint32 { return u.Enc }),
	sortDec:      byGPU(func(u *gpu.ProcUsage) uint32 { return u.Dec }),
	sortGPUTime:  byGPU(func(u *gpu.ProcUsage) time.Duration { return u.GPUTime }),
}

func (m *Model) sortRows(rows []row) {
	slices.SortStableFunc(rows, func(a, b row) int {
		c := comparators[m.sortBy](a, b)
		if m.sortDesc {
			c = -c
		}
		if c == 0 {
			c = cmp.Compare(a.PID, b.PID)
		}
		return c
	})
}

func (m *Model) visible(p *sysinfo.Process) bool {
	if p.Kernel && !m.showKernel {
		return false
	}
	if p.Thread && (!m.showThreads || m.tab == tabGPU) {
		return false
	}
	if m.tab == tabGPU {
		if _, ok := m.gpus.Procs[p.PID]; !ok {
			return false
		}
	}
	if m.filter != "" {
		needle := strings.ToLower(m.filter)
		if !strings.Contains(strings.ToLower(p.Command), needle) && !strings.Contains(strings.ToLower(p.Comm), needle) {
			return false
		}
	}
	return true
}

func (m *Model) buildRows() []row {
	rows := make([]row, 0, len(m.sys.Procs))
	for i := range m.sys.Procs {
		p := &m.sys.Procs[i]
		if !m.visible(p) {
			continue
		}
		r := row{Process: p}
		if !p.Thread {
			if u, ok := m.gpus.Procs[p.PID]; ok {
				r.gpu = &u
			}
		}
		rows = append(rows, r)
	}
	m.sortRows(rows)
	if m.tree {
		rows = treeOrder(rows)
	}
	return rows
}

func treeOrder(rows []row) []row {
	present := make(map[int]bool, len(rows))
	for _, r := range rows {
		present[r.PID] = true
	}
	children := make(map[int][]row)
	var roots []row
	for _, r := range rows {
		if r.PPID != r.PID && present[r.PPID] {
			children[r.PPID] = append(children[r.PPID], r)
			continue
		}
		roots = append(roots, r)
	}
	out := make([]row, 0, len(rows))
	var walk func(r row, indent string, last, root bool)
	walk = func(r row, indent string, last, root bool) {
		next := indent
		if !root {
			branch := "├─ "
			next = indent + "│  "
			if last {
				branch = "└─ "
				next = indent + "   "
			}
			r.prefix = indent + branch
		}
		out = append(out, r)
		kids := children[r.PID]
		for i, k := range kids {
			walk(k, next, i == len(kids)-1, false)
		}
	}
	for _, r := range roots {
		walk(r, "", true, true)
	}
	return out
}
