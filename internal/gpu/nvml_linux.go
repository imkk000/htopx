package gpu

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Monitor struct {
	devices  []nvml.Device
	lastSeen []uint64
	driver   string
	cuda     string
}

func Open() (*Monitor, error) {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("nvml init: %w", ret)
	}
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		_ = nvml.Shutdown()
		return nil, fmt.Errorf("nvml device count: %w", ret)
	}
	m := &Monitor{lastSeen: make([]uint64, count)}
	for i := range count {
		d, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			continue
		}
		m.devices = append(m.devices, d)
	}
	if len(m.devices) == 0 {
		_ = nvml.Shutdown()
		return nil, errors.New("no nvidia devices")
	}
	m.driver, _ = nvml.SystemGetDriverVersion()
	if v, ret := nvml.SystemGetCudaDriverVersion(); ret == nvml.SUCCESS {
		m.cuda = fmt.Sprintf("%d.%d", v/1000, v%1000/10)
	}
	return m, nil
}

func (m *Monitor) Close() {
	if m != nil {
		_ = nvml.Shutdown()
	}
}

func (m *Monitor) Sample() Snapshot {
	snap := Snapshot{Procs: make(map[int]ProcUsage)}
	if m == nil {
		return snap
	}
	snap.Kind, snap.Driver, snap.CUDA = KindNVIDIA, m.driver, m.cuda
	for i, d := range m.devices {
		snap.Devices = append(snap.Devices, readDevice(i, d))
		m.readProcs(i, d, snap.Procs)
	}
	return snap
}

func readDevice(i int, d nvml.Device) Device {
	dev := Device{Index: i, PState: -1}
	dev.Name, _ = d.GetName()
	if pci, ret := d.GetPciInfo(); ret == nvml.SUCCESS {
		dev.BusID = cString(pci.BusId[:])
	}
	if u, ret := d.GetUtilizationRates(); ret == nvml.SUCCESS {
		dev.Util, dev.MemUtil = u.Gpu, u.Memory
	}
	if mem, ret := d.GetMemoryInfo(); ret == nvml.SUCCESS {
		dev.MemUsed, dev.MemTotal = mem.Used, mem.Total
	}
	dev.Temp, _ = d.GetTemperature(nvml.TEMPERATURE_GPU)
	if fan, ret := d.GetFanSpeed(); ret == nvml.SUCCESS {
		dev.Fan, dev.HasFan = fan, true
	}
	if p, ret := d.GetPowerUsage(); ret == nvml.SUCCESS {
		dev.PowerW = float64(p) / 1000
	}
	if p, ret := d.GetEnforcedPowerLimit(); ret == nvml.SUCCESS {
		dev.PowerCapW = float64(p) / 1000
	}
	if ps, ret := d.GetPerformanceState(); ret == nvml.SUCCESS {
		dev.PState = int(ps)
	}
	dev.ClockSM, _ = d.GetClockInfo(nvml.CLOCK_SM)
	dev.ClockMem, _ = d.GetClockInfo(nvml.CLOCK_MEM)
	dev.ClockSMMax, _ = d.GetMaxClockInfo(nvml.CLOCK_SM)
	dev.ClockMemMax, _ = d.GetMaxClockInfo(nvml.CLOCK_MEM)
	dev.EncUtil, _, _ = d.GetEncoderUtilization()
	dev.DecUtil, _, _ = d.GetDecoderUtilization()
	return dev
}

func (m *Monitor) readProcs(i int, d nvml.Device, out map[int]ProcUsage) {
	add := func(infos []nvml.ProcessInfo, kind string) {
		for _, info := range infos {
			pid := int(info.Pid)
			u, seen := out[pid]
			u.GPU = i
			switch {
			case !seen:
				u.Type = kind
			case u.Type != kind && !strings.Contains(u.Type, kind):
				u.Type = "C+G"
			}
			if info.UsedGpuMemory != ^uint64(0) {
				u.MemUsed += info.UsedGpuMemory
			}
			out[pid] = u
		}
	}
	if infos, ret := d.GetComputeRunningProcesses(); ret == nvml.SUCCESS {
		add(infos, "C")
	}
	if infos, ret := d.GetGraphicsRunningProcesses(); ret == nvml.SUCCESS {
		add(infos, "G")
	}
	samples, ret := d.GetProcessUtilization(m.lastSeen[i])
	if ret != nvml.SUCCESS {
		return
	}
	latest := make(map[int]nvml.ProcessUtilizationSample, len(samples))
	for _, s := range samples {
		if s.TimeStamp > m.lastSeen[i] {
			m.lastSeen[i] = s.TimeStamp
		}
		if prev, ok := latest[int(s.Pid)]; !ok || s.TimeStamp > prev.TimeStamp {
			latest[int(s.Pid)] = s
		}
	}
	for pid, s := range latest {
		u, ok := out[pid]
		if !ok {
			continue
		}
		u.SM, u.Mem, u.Enc, u.Dec = s.SmUtil, s.MemUtil, s.EncUtil, s.DecUtil
		out[pid] = u
	}
}

func cString(b []int8) string {
	runes := make([]rune, 0, len(b))
	for _, c := range b {
		if c <= 0 {
			break
		}
		runes = append(runes, rune(c))
	}
	return string(runes)
}
