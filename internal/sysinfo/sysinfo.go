package sysinfo

import (
	"os/user"
	"strconv"
	"time"
)

const ClockTicks = 100

type cpuTicks struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

func (c cpuTicks) total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

type CPUUsage struct {
	User, Nice, System, Steal, Total float64
}

type MemInfo struct {
	Total, Free, Used, Buffers, Cache, Shared uint64
	SwapTotal, SwapUsed, SwapCache            uint64
}

type LoadAvg [3]float64

type Process struct {
	PID, PPID, TGID int
	Kernel, Thread  bool
	UID             uint32
	User            string
	Comm, Command   string
	State           byte
	Priority, Nice  int
	Threads         int
	Virt, Res, Shr  uint64
	Ticks           uint64
	CPU, Mem        float64
}

type Snapshot struct {
	CPU           CPUUsage
	Cores         []CPUUsage
	Mem           MemInfo
	Load          LoadAvg
	Uptime        time.Duration
	Procs         []Process
	Tasks         int
	UserThreads   int
	KernelThreads int
	Running       int
}

type tickKey struct {
	id    int
	start uint64
}

type Sampler struct {
	plat      *platform
	platErr   error
	prevTotal cpuTicks
	prevCores []cpuTicks
	prevTicks map[tickKey]uint64
	prevAt    time.Time
	users     map[uint32]string
}

func NewSampler() *Sampler {
	plat, err := newPlatform()
	return &Sampler{
		plat:      plat,
		platErr:   err,
		prevTicks: make(map[tickKey]uint64),
		users:     make(map[uint32]string),
	}
}

func (s *Sampler) Sample() (Snapshot, error) {
	if s.platErr != nil {
		return Snapshot{}, s.platErr
	}
	now := time.Now()
	total, cores, err := s.plat.cpuTimes()
	if err != nil {
		return Snapshot{}, err
	}
	mem, err := s.plat.memInfo()
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{
		CPU:    cpuUsage(s.prevTotal, total),
		Cores:  make([]CPUUsage, len(cores)),
		Mem:    mem,
		Load:   s.plat.loadAvg(),
		Uptime: s.plat.uptime(),
	}
	for i, c := range cores {
		var prev cpuTicks
		if i < len(s.prevCores) {
			prev = s.prevCores[i]
		}
		snap.Cores[i] = cpuUsage(prev, c)
	}

	var elapsed float64
	if !s.prevAt.IsZero() {
		elapsed = now.Sub(s.prevAt).Seconds()
	}
	ticks := make(map[tickKey]uint64, len(s.prevTicks))
	s.collect(&snap, ticks, elapsed)

	s.prevTotal, s.prevCores, s.prevTicks, s.prevAt = total, cores, ticks, now
	return snap, nil
}

func (snap *Snapshot) countRunning(p Process) {
	if p.State == 'R' {
		snap.Running++
	}
}

func (s *Sampler) finish(p *Process, start uint64, ticks map[tickKey]uint64, elapsed float64, memTotal uint64) {
	key := tickKey{p.PID, start}
	ticks[key] = p.Ticks
	if prev, ok := s.prevTicks[key]; ok && elapsed > 0 && p.Ticks >= prev {
		p.CPU = float64(p.Ticks-prev) / (elapsed * ClockTicks) * 100
	}
	if memTotal > 0 {
		p.Mem = float64(p.Res) / float64(memTotal) * 100
	}
}

func (s *Sampler) userName(uid uint32) string {
	if name, ok := s.users[uid]; ok {
		return name
	}
	id := strconv.FormatUint(uint64(uid), 10)
	name := id
	if u, err := user.LookupId(id); err == nil {
		name = u.Username
	}
	s.users[uid] = name
	return name
}

func cpuUsage(prev, cur cpuTicks) CPUUsage {
	span := float64(cur.total() - prev.total())
	if span <= 0 || cur.total() < prev.total() {
		return CPUUsage{}
	}
	pct := func(a, b uint64) float64 {
		if a < b {
			return 0
		}
		return float64(a-b) / span * 100
	}
	u := CPUUsage{
		User:   pct(cur.User, prev.User),
		Nice:   pct(cur.Nice, prev.Nice),
		System: pct(cur.System+cur.IRQ+cur.SoftIRQ, prev.System+prev.IRQ+prev.SoftIRQ),
		Steal:  pct(cur.Steal, prev.Steal),
	}
	u.Total = min(u.User+u.Nice+u.System+u.Steal, 100)
	return u
}
