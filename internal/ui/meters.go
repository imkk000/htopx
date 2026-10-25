package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/imkk000/htopx/internal/gpu"
	"github.com/imkk000/htopx/internal/sysinfo"
)

type segment struct {
	frac  float64
	color string
}

func meter(label string, labelW, width int, segs []segment, text string) string {
	inner := width - labelW - 2
	if inner < 1 {
		return padRight("", width)
	}
	chars := make([]byte, inner)
	colors := make([]string, inner)
	for i := range chars {
		chars[i] = ' '
	}
	var acc float64
	var pos int
	for _, s := range segs {
		acc += max(s.frac, 0)
		end := min(int(math.Round(acc*float64(inner))), inner)
		for ; pos < end; pos++ {
			chars[pos], colors[pos] = '|', s.color
		}
	}
	if len(text) > inner {
		text = text[len(text)-inner:]
	}
	for i := range len(text) {
		at := inner - len(text) + i
		chars[at] = text[i]
		if colors[at] == "" {
			colors[at] = cDim
		}
	}

	parts := []string{paint(cCyan, padLeft(label, labelW)), paint(cBold, "[")}
	start := 0
	for i := 1; i <= inner; i++ {
		if i == inner || colors[i] != colors[start] {
			run := string(chars[start:i])
			if colors[start] != "" {
				run = paint(colors[start], run)
			}
			parts = append(parts, run)
			start = i
		}
	}
	parts = append(parts, paint(cBold, "]"))
	return strings.Join(parts, "")
}

func pct(v float64) float64 { return v / 100 }

func ratio(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func cpuMeter(label string, labelW, width int, u sysinfo.CPUUsage) string {
	segs := []segment{
		{pct(u.Nice), cBlue},
		{pct(u.User), cGreen},
		{pct(u.System), cRed},
		{pct(u.Steal), cCyan},
	}
	return meter(label, labelW, width, segs, fmt.Sprintf("%.1f%%", u.Total))
}

func memMeter(width int, m sysinfo.MemInfo) string {
	segs := []segment{
		{ratio(m.Used, m.Total), cGreen},
		{ratio(m.Shared, m.Total), cMagenta},
		{ratio(m.Buffers, m.Total), cBlue},
		{ratio(m.Cache, m.Total), cYellow},
	}
	return meter("Mem", 3, width, segs, humanKiB(m.Used)+"/"+humanKiB(m.Total))
}

func swapMeter(width int, m sysinfo.MemInfo) string {
	segs := []segment{
		{ratio(m.SwapUsed, m.SwapTotal), cRed},
		{ratio(m.SwapCache, m.SwapTotal), cYellow},
	}
	return meter("Swp", 3, width, segs, humanKiB(m.SwapUsed)+"/"+humanKiB(m.SwapTotal))
}

func gpuMeters(kind gpu.Kind, width int, d gpu.Device) [3]string {
	if kind == gpu.KindApple {
		return appleMeters(width, d)
	}
	util := meter("GPU", 3, width, []segment{{pct(float64(d.Util)), cGreen}}, fmt.Sprintf("%d%%", d.Util))
	vram := meter("VRM", 3, width, []segment{{ratio(d.MemUsed, d.MemTotal), cYellow}},
		humanKiB(d.MemUsed/1024)+"/"+humanKiB(d.MemTotal/1024))
	power := meter("PWR", 3, width, []segment{{d.PowerW / max(d.PowerCapW, 1), cRed}},
		fmt.Sprintf("%.0fW/%.0fW", d.PowerW, d.PowerCapW))
	return [3]string{util, vram, power}
}

func appleMeters(width int, d gpu.Device) [3]string {
	util := meter("GPU", 3, width, []segment{{pct(float64(d.Util)), cGreen}}, fmt.Sprintf("%d%%", d.Util))
	renderer := meter("REN", 3, width, []segment{{pct(float64(d.Renderer)), cYellow}}, fmt.Sprintf("%d%%", d.Renderer))
	mem := meter("MEM", 3, width, []segment{{ratio(d.MemUsed, d.MemTotal), cMagenta}},
		humanKiB(d.MemUsed/1024)+"/"+humanKiB(d.MemTotal/1024))
	return [3]string{util, renderer, mem}
}

func kv(k, v string) string { return paint(cCyan, k+": ") + paint(cBold, v) }

func gpuInfo(kind gpu.Kind, d gpu.Device, prefix string) [3]string {
	name := paint(cCyan, prefix) + paint(cBold, d.Name)
	if kind == gpu.KindApple {
		if d.Cores > 0 {
			name += "  " + paint(cDim, strconv.Itoa(d.Cores)+"-core GPU")
		}
		return [3]string{
			name,
			kv("Tiler", fmt.Sprintf("%d%%", d.Tiler)),
			kv("Unified memory in use", humanKiB(d.MemUsed/1024)) + "  " + kv("Allocated", humanKiB(d.MemAlloc/1024)),
		}
	}
	if d.BusID != "" {
		name += "  " + paint(cDim, d.BusID)
	}
	fan := "N/A"
	if d.HasFan {
		fan = fmt.Sprintf("%d%%", d.Fan)
	}
	perf := "N/A"
	if d.PState >= 0 {
		perf = fmt.Sprintf("P%d", d.PState)
	}
	status := paint(cCyan, "Temp: ") + paint(tempColor(float64(d.Temp), 70, 85), fmt.Sprintf("%dC", d.Temp)) + "  " +
		kv("Fan", fan) + "  " + kv("Perf", perf) + "  " +
		kv("Enc", fmt.Sprintf("%d%%", d.EncUtil)) + "  " + kv("Dec", fmt.Sprintf("%d%%", d.DecUtil))
	clocks := kv("SM", fmt.Sprintf("%d/%dMHz", d.ClockSM, d.ClockSMMax)) + "  " +
		kv("Mem", fmt.Sprintf("%d/%dMHz", d.ClockMem, d.ClockMemMax)) + "  " +
		kv("MemBW", fmt.Sprintf("%d%%", d.MemUtil))
	return [3]string{name, status, clocks}
}

func tempColor(t, warn, crit float64) string {
	switch {
	case t >= crit:
		return cBoldRed
	case t >= warn:
		return "1;" + cYellow
	}
	return cBoldGreen
}

func sensorsLine(s sysinfo.Sensors) string {
	var parts []string
	if s.HasCPUTemp {
		parts = append(parts, paint(cCyan, "CPU: ")+paint(tempColor(s.CPUTemp, 70, 85), fmt.Sprintf("%.0fC", s.CPUTemp)))
	}
	if s.HasCPUFan {
		parts = append(parts, kv("Fan", fmt.Sprintf("%drpm", s.CPUFanRPM)))
	}
	if s.HasRAMTemp {
		parts = append(parts, paint(cCyan, "RAM: ")+paint(tempColor(s.RAMTemp, 55, 75), fmt.Sprintf("%.0fC", s.RAMTemp)))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) headerLines() []string {
	w := m.width
	snap := m.sys
	var lines []string

	cols := 1
	switch {
	case w >= 120:
		cols = 4
	case w >= 60:
		cols = 2
	}
	n := len(snap.Cores)
	if n > 0 {
		cols = min(cols, n)
		rows := (n + cols - 1) / cols
		meterW := (w - 2 - (cols - 1)) / cols
		labelW := max(3, len(strconv.Itoa(n-1)))
		for r := range rows {
			parts := make([]string, 0, cols)
			for c := range cols {
				i := c*rows + r
				if i >= n {
					break
				}
				parts = append(parts, cpuMeter(strconv.Itoa(i), labelW, meterW, snap.Cores[i]))
			}
			lines = append(lines, " "+strings.Join(parts, " "))
		}
	}

	half := (w - 3) / 2
	tasks := paint(cCyan, "Tasks: ") + paint(cBold, strconv.Itoa(snap.Tasks)) + paint(cCyan, ", ") +
		paint(cBoldGreen, strconv.Itoa(snap.UserThreads)) + paint(cCyan, " thr")
	if snap.KernelThreads > 0 {
		tasks += paint(cDim, ", "+strconv.Itoa(snap.KernelThreads)+" kthr")
	}
	tasks += paint(cCyan, "; ") + paint(cBoldGreen, strconv.Itoa(snap.Running)) + paint(cCyan, " running")
	load := paint(cCyan, "Load average: ") + paint(cBold, fmt.Sprintf("%.2f ", snap.Load[0])) +
		paint(cCyan, fmt.Sprintf("%.2f ", snap.Load[1])) + paint(cCyan, fmt.Sprintf("%.2f", snap.Load[2]))
	uptime := paint(cCyan, "Uptime: ") + paint(cBold, formatUptime(int64(snap.Uptime.Seconds())))
	var driver string
	if m.gpus.Driver != "" {
		driver = paint(cCyan, "Driver: ") + paint(cBold, m.gpus.Driver)
		if m.gpus.CUDA != "" {
			driver += paint(cCyan, "  CUDA: ") + paint(cBold, m.gpus.CUDA)
		}
	}

	pair := func(left, right string) string {
		return " " + fit(left, half) + "  " + right
	}
	lines = append(lines,
		pair(memMeter(half, snap.Mem), tasks),
		pair(swapMeter(half, snap.Mem), load),
		pair(sensorsLine(snap.Sensors), uptime),
	)
	if driver != "" {
		lines = append(lines, pair(driver, ""))
	}
	for _, d := range m.gpus.Devices {
		var prefix string
		if len(m.gpus.Devices) > 1 {
			prefix = "GPU " + strconv.Itoa(d.Index) + ": "
		}
		bars, info := gpuMeters(m.gpus.Kind, half, d), gpuInfo(m.gpus.Kind, d, prefix)
		for i := range 3 {
			lines = append(lines, pair(bars[i], info[i]))
		}
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return lines
}
