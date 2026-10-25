package sysinfo

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"time"
)

func (*platform) cpuTimes() (cpuTicks, []cpuTicks, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTicks{}, nil, err
	}
	var total cpuTicks
	var cores []cpuTicks
	for line := range bytes.Lines(data) {
		if !bytes.HasPrefix(line, []byte("cpu")) {
			break
		}
		fields := strings.Fields(string(line))
		var vals [8]uint64
		for i := 0; i < len(vals) && i+1 < len(fields); i++ {
			vals[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
		}
		t := cpuTicks{
			User: vals[0], Nice: vals[1], System: vals[2], Idle: vals[3],
			IOWait: vals[4], IRQ: vals[5], SoftIRQ: vals[6], Steal: vals[7],
		}
		if fields[0] == "cpu" {
			total = t
			continue
		}
		cores = append(cores, t)
	}
	return total, cores, nil
}

func (*platform) memInfo() (MemInfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	vals := make(map[string]uint64)
	for line := range strings.Lines(string(data)) {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		vals[key], _ = strconv.ParseUint(fields[0], 10, 64)
	}
	m := MemInfo{
		Total:     vals["MemTotal"],
		Free:      vals["MemFree"],
		Buffers:   vals["Buffers"],
		Shared:    vals["Shmem"],
		SwapTotal: vals["SwapTotal"],
		SwapCache: vals["SwapCached"],
	}
	cache := vals["Cached"] + vals["SReclaimable"]
	if cache > m.Shared {
		m.Cache = cache - m.Shared
	}
	if used := m.Free + m.Buffers + m.Cache + m.Shared; m.Total > used {
		m.Used = m.Total - used
	}
	if swapUsed := vals["SwapFree"] + m.SwapCache; m.SwapTotal > swapUsed {
		m.SwapUsed = m.SwapTotal - swapUsed
	}
	return m, nil
}

func (*platform) loadAvg() LoadAvg {
	var l LoadAvg
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return l
	}
	fields := strings.Fields(string(data))
	for i := 0; i < 3 && i < len(fields); i++ {
		l[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return l
}

func (*platform) uptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return time.Duration(secs * float64(time.Second))
}
