package sysinfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinStateStopped = 4
	darwinStateZombie  = 5
)

type platform struct {
	mach     machLib
	pageKiB  uint64
	memTotal uint64
	boot     time.Time
	commands map[tickKey]string
}

func newPlatform() (*platform, error) {
	mach, err := loadMach()
	if err != nil {
		return nil, err
	}
	memBytes, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return nil, fmt.Errorf("hw.memsize: %w", err)
	}
	p := &platform{
		mach:     mach,
		pageKiB:  4,
		memTotal: memBytes / 1024,
		commands: make(map[tickKey]string),
	}
	if size := os.Getpagesize(); size > 0 {
		p.pageKiB = uint64(size) / 1024
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		p.boot = time.Unix(tv.Unix())
	}
	return p, nil
}

func (p *platform) cpuTimes() (cpuTicks, []cpuTicks, error) {
	var count, infoCount uint32
	var info *uint32
	if kr := p.mach.hostProcessorInfo(p.mach.host, processorCPULoadInfo, &count, &info, &infoCount); kr != 0 || info == nil {
		return cpuTicks{}, nil, fmt.Errorf("host_processor_info: kern_return %d", kr)
	}
	defer p.mach.vmDeallocate(p.mach.task, uintptr(unsafe.Pointer(info)), uintptr(infoCount)*4)
	ticks := unsafe.Slice(info, int(count)*4)
	var total cpuTicks
	cores := make([]cpuTicks, count)
	for i := range cores {
		c := cpuTicks{
			User:   uint64(ticks[i*4]),
			System: uint64(ticks[i*4+1]),
			Idle:   uint64(ticks[i*4+2]),
			Nice:   uint64(ticks[i*4+3]),
		}
		cores[i] = c
		total.User += c.User
		total.System += c.System
		total.Idle += c.Idle
		total.Nice += c.Nice
	}
	return total, cores, nil
}

func (p *platform) memInfo() (MemInfo, error) {
	var vm [64]uint32
	count := uint32(hostVMInfo64Count)
	if kr := p.mach.hostStatistics64(p.mach.host, hostVMInfo64, &vm[0], &count); kr != 0 {
		return MemInfo{}, fmt.Errorf("host_statistics64: kern_return %d", kr)
	}
	pages := func(n uint32) uint64 { return uint64(n) * p.pageKiB }
	free, wired, purgeable := vm[0], vm[3], vm[22]
	compressed, external, internal := vm[32], vm[34], vm[35]
	m := MemInfo{
		Total: p.memTotal,
		Free:  pages(free),
		Used:  pages(internal-min(purgeable, internal)) + pages(wired) + pages(compressed),
		Cache: pages(external) + pages(purgeable),
	}
	if raw, err := unix.SysctlRaw("vm.swapusage"); err == nil && len(raw) >= 24 {
		m.SwapTotal = binary.LittleEndian.Uint64(raw[0:]) / 1024
		m.SwapUsed = binary.LittleEndian.Uint64(raw[16:]) / 1024
	}
	return m, nil
}

func (*platform) loadAvg() LoadAvg {
	var l LoadAvg
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 24 {
		return l
	}
	scale := float64(binary.LittleEndian.Uint64(raw[16:]))
	if scale == 0 {
		return l
	}
	for i := range l {
		l[i] = float64(binary.LittleEndian.Uint32(raw[i*4:])) / scale
	}
	return l
}

func (p *platform) uptime() time.Duration {
	if p.boot.IsZero() {
		return 0
	}
	return time.Since(p.boot)
}

func (p *platform) taskInfo(pid int32) (procTaskInfo, bool) {
	var ti procTaskInfo
	size := int32(unsafe.Sizeof(ti))
	n := p.mach.procPidInfo(pid, procPIDTaskInfo, 0, &ti, size)
	return ti, n == size
}

func (p *platform) machToTicks(t uint64) uint64 {
	ns := t * uint64(p.mach.timebase.Numer) / uint64(p.mach.timebase.Denom)
	return ns / uint64(time.Second/ClockTicks)
}

func (p *platform) command(key tickKey, comm string, next map[tickKey]string) string {
	cmd, ok := p.commands[key]
	if !ok {
		cmd = procArgs(key.id, comm)
	}
	next[key] = cmd
	return cmd
}

func (s *Sampler) collect(snap *Snapshot, ticks map[tickKey]uint64, elapsed float64) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return
	}
	commands := make(map[tickKey]string, len(kps))
	for i := range kps {
		kp := &kps[i]
		pid := int(kp.Proc.P_pid)
		p := Process{
			PID:     pid,
			PPID:    int(kp.Eproc.Ppid),
			TGID:    pid,
			UID:     kp.Eproc.Ucred.Uid,
			Comm:    cString(kp.Proc.P_comm[:]),
			Nice:    int(kp.Proc.P_nice),
			Threads: 1,
			State:   'S',
		}
		p.User = s.userName(p.UID)
		if ti, ok := s.plat.taskInfo(kp.Proc.P_pid); ok {
			p.Virt = ti.VirtualSize / 1024
			p.Res = ti.ResidentSize / 1024
			p.Ticks = s.plat.machToTicks(ti.TotalUser + ti.TotalSystem)
			p.Threads = int(ti.ThreadNum)
			p.Priority = int(ti.Priority)
			if ti.NumRunning > 0 {
				p.State = 'R'
			}
		}
		switch kp.Proc.P_stat {
		case darwinStateStopped:
			p.State = 'T'
		case darwinStateZombie:
			p.State = 'Z'
		}
		start := startKey(kp.Proc.P_starttime)
		p.Command = s.plat.command(tickKey{pid, start}, p.Comm, commands)
		s.finish(&p, start, ticks, elapsed, snap.Mem.Total)

		snap.Procs = append(snap.Procs, p)
		snap.Tasks++
		snap.UserThreads += p.Threads
		snap.countRunning(p)
	}
	s.plat.commands = commands
}

func startKey(tv unix.Timeval) uint64 {
	if tv.Sec < 0 || tv.Usec < 0 {
		return 0
	}
	return uint64(tv.Sec)*1_000_000 + uint64(tv.Usec)
}

func procArgs(pid int, comm string) string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return comm
	}
	argc := int(binary.LittleEndian.Uint32(raw))
	rest := raw[4:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return comm
	}
	rest = bytes.TrimLeft(rest[end:], "\x00")
	args := make([]string, 0, argc)
	for len(args) < argc && len(rest) > 0 {
		end = bytes.IndexByte(rest, 0)
		if end < 0 {
			end = len(rest)
		}
		args = append(args, string(rest[:end]))
		rest = rest[min(end+1, len(rest)):]
	}
	if len(args) == 0 {
		return comm
	}
	return strings.Join(args, " ")
}

func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
