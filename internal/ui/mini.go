package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/imkk000/htopx/internal/gpu"
	"github.com/imkk000/htopx/internal/sysinfo"
)

const miniTopMax = 10

type topRow struct {
	name, value string
}

type topList struct {
	title string
	rows  []topRow
}

func (m *Model) miniLines() []string {
	snap := m.sys
	half := (m.width - 3) / 2
	lines := make([]string, 0, 6+2*len(m.gpus.Devices))
	lines = append(lines,
		pair(half, cpuMeter("CPU", 3, half, snap.CPU), kv("Load", fmt.Sprintf("%.2f %.2f %.2f", snap.Load[0], snap.Load[1], snap.Load[2]))),
		pair(half, memMeter(half, snap.Mem), sensorsLine(snap.Sensors)),
		pair(half, swapMeter(half, snap.Mem), kv("Up", compactUptime(snap.Uptime))),
	)
	for _, d := range m.gpus.Devices {
		lines = append(lines, m.miniGPU(half, d)...)
	}
	lines = append(lines, m.topLines()...)
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "")
	}
	return lines
}

func compactUptime(d time.Duration) string {
	secs := int64(d.Seconds())
	clock := fmt.Sprintf("%02d:%02d", secs/3600%24, secs/60%60)
	if days := secs / 86400; days > 0 {
		return strconv.FormatInt(days, 10) + "d " + clock
	}
	return clock
}

func (m *Model) miniGPU(half int, d gpu.Device) []string {
	bars := gpuMeters(m.gpus.Kind, half, d)
	name := d.Name
	if len(m.gpus.Devices) > 1 {
		name = strconv.Itoa(d.Index) + ": " + name
	}
	if m.gpus.Kind == gpu.KindApple {
		status := kv("Ren", fmt.Sprintf("%d%%", d.Renderer)) + "  " + kv("Tiler", fmt.Sprintf("%d%%", d.Tiler))
		return []string{pair(half, bars[0], status), pair(half, bars[2], paint(cBold, name))}
	}
	status := paint(tempColor(float64(d.Temp), 70, 85), fmt.Sprintf("%dC", d.Temp)) + "  " +
		kv("Pwr", fmt.Sprintf("%.0fW/%.0fW", d.PowerW, d.PowerCapW))
	if d.HasFan {
		status += "  " + kv("Fan", fmt.Sprintf("%d%%", d.Fan))
	}
	return []string{pair(half, bars[0], status), pair(half, bars[1], paint(cBold, name))}
}

func (m *Model) topLines() []string {
	lists := m.topLists(miniTopMax)
	lines := make([]string, 0, len(lists))
	for _, l := range lists {
		var line strings.Builder
		line.WriteString(" ")
		line.WriteString(paint(cCyan, l.title))
		width := 1 + len(l.title)
		for _, r := range l.rows {
			width += 2 + ansi.StringWidth(r.name) + 1 + ansi.StringWidth(r.value)
			if width > m.width {
				break
			}
			line.WriteString("  ")
			line.WriteString(r.name)
			line.WriteString(" ")
			line.WriteString(paint(cBold, r.value))
		}
		lines = append(lines, line.String())
	}
	return lines
}

func (m *Model) topLists(n int) []topList {
	procs := make([]*sysinfo.Process, 0, len(m.sys.Procs))
	for i := range m.sys.Procs {
		if p := &m.sys.Procs[i]; !p.Thread {
			procs = append(procs, p)
		}
	}
	lists := []topList{
		{"CPU", rankProcs(procs, n, func(p *sysinfo.Process) float64 { return p.CPU }, func(p *sysinfo.Process) string {
			return fmt.Sprintf("%.1f%%", p.CPU)
		})},
		{"MEM", rankProcs(procs, n, func(p *sysinfo.Process) float64 { return float64(p.Res) }, func(p *sysinfo.Process) string {
			return humanKiB(p.Res)
		})},
	}
	if m.gpus.Kind != gpu.KindNone {
		lists = append(lists, m.topGPU(procs, n))
	}
	return lists
}

func rankProcs(procs []*sysinfo.Process, n int, key func(p *sysinfo.Process) float64, format func(p *sysinfo.Process) string) []topRow {
	slices.SortFunc(procs, func(a, b *sysinfo.Process) int {
		return cmp.Or(cmp.Compare(key(b), key(a)), cmp.Compare(a.PID, b.PID))
	})
	rows := make([]topRow, 0, n)
	for _, p := range procs[:min(n, len(procs))] {
		if key(p) <= 0 {
			break
		}
		rows = append(rows, topRow{p.Comm, format(p)})
	}
	return rows
}

func (m *Model) topGPU(procs []*sysinfo.Process, n int) topList {
	type entry struct {
		proc  *sysinfo.Process
		usage gpu.ProcUsage
	}
	var entries []entry
	for _, p := range procs {
		if u, ok := m.gpus.Procs[p.PID]; ok && (u.SM > 0 || u.MemUsed > 0) {
			entries = append(entries, entry{p, u})
		}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return cmp.Or(cmp.Compare(b.usage.SM, a.usage.SM), cmp.Compare(b.usage.MemUsed, a.usage.MemUsed),
			cmp.Compare(a.proc.PID, b.proc.PID))
	})
	list := topList{title: gpuLabel}
	for _, e := range entries[:min(n, len(entries))] {
		value := fmt.Sprintf("%d%%", e.usage.SM)
		if m.gpus.Kind == gpu.KindNVIDIA {
			value += "/" + humanKiB(e.usage.MemUsed/1024)
		}
		list.rows = append(list.rows, topRow{e.proc.Comm, value})
	}
	return list
}
