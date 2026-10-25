package gpu

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

var (
	reModel     = regexp.MustCompile(`"model" = "([^"]*)"`)
	reCores     = regexp.MustCompile(`"gpu-core-count" = (\d+)`)
	reStat      = regexp.MustCompile(`"([^"]+)"=(\d+)`)
	reCreator   = regexp.MustCompile(`"IOUserClientCreator" = "pid (\d+),`)
	reGPUTime   = regexp.MustCompile(`"accumulatedGPUTime"=(\d+)`)
	entrySplit  = []byte("+-o ")
	perfStatKey = []byte(`"PerformanceStatistics"`)
)

type Monitor struct {
	memTotal uint64
	prevTime map[int]time.Duration
	prevAt   time.Time
}

func Open() (*Monitor, error) {
	if _, err := exec.LookPath("ioreg"); err != nil {
		return nil, fmt.Errorf("ioreg: %w", err)
	}
	memTotal, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return nil, fmt.Errorf("hw.memsize: %w", err)
	}
	m := &Monitor{memTotal: memTotal, prevTime: make(map[int]time.Duration)}
	if len(m.devices()) == 0 {
		return nil, errors.New("no apple gpu found")
	}
	return m, nil
}

func (*Monitor) Close() {}

func (m *Monitor) Sample() Snapshot {
	snap := Snapshot{Procs: make(map[int]ProcUsage)}
	if m == nil {
		return snap
	}
	snap.Kind = KindApple
	snap.Devices = m.devices()
	m.readProcs(snap.Procs)
	return snap
}

func ioreg(cmd *exec.Cmd) [][]byte {
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return bytes.Split(out, entrySplit)
}

func (m *Monitor) devices() []Device {
	var devices []Device
	for _, entry := range ioreg(exec.Command("ioreg", "-r", "-w", "0", "-d", "1", "-c", "IOAccelerator")) {
		if !bytes.Contains(entry, perfStatKey) {
			continue
		}
		stats := make(map[string]uint64)
		for _, match := range reStat.FindAllSubmatch(entry, -1) {
			stats[string(match[1])], _ = strconv.ParseUint(string(match[2]), 10, 64)
		}
		d := Device{
			Index:    len(devices),
			Name:     "Apple GPU",
			PState:   -1,
			Util:     clampPercent(stats["Device Utilization %"]),
			Renderer: clampPercent(stats["Renderer Utilization %"]),
			Tiler:    clampPercent(stats["Tiler Utilization %"]),
			MemUsed:  stats["In use system memory"],
			MemAlloc: stats["Alloc system memory"],
			MemTotal: m.memTotal,
		}
		if match := reModel.FindSubmatch(entry); match != nil {
			d.Name = string(match[1])
		}
		if match := reCores.FindSubmatch(entry); match != nil {
			d.Cores, _ = strconv.Atoi(string(match[1]))
		}
		devices = append(devices, d)
	}
	return devices
}

func (m *Monitor) readProcs(out map[int]ProcUsage) {
	now := time.Now()
	totals := make(map[int]time.Duration)
	for _, entry := range ioreg(exec.Command("ioreg", "-r", "-w", "0", "-l", "-c", "AGXDeviceUserClient")) {
		match := reCreator.FindSubmatch(entry)
		if match == nil {
			continue
		}
		pid, err := strconv.Atoi(string(match[1]))
		if err != nil {
			continue
		}
		total := totals[pid]
		for _, t := range reGPUTime.FindAllSubmatch(entry, -1) {
			ns, _ := strconv.ParseInt(string(t[1]), 10, 64)
			total += time.Duration(ns)
		}
		totals[pid] = total
	}
	elapsed := now.Sub(m.prevAt)
	for pid, total := range totals {
		u := ProcUsage{Type: "G", GPUTime: total}
		if prev, ok := m.prevTime[pid]; ok && !m.prevAt.IsZero() && elapsed > 0 && total >= prev {
			u.SM = clampPercent(uint64(float64(total-prev) / float64(elapsed) * 100))
		}
		out[pid] = u
	}
	m.prevTime, m.prevAt = totals, now
}

func clampPercent(v uint64) uint32 {
	return uint32(min(v, 100))
}
