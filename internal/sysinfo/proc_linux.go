package sysinfo

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const kernelThreadFlag = 0x00200000

type platform struct {
	pageKiB uint64
}

func newPlatform() (*platform, error) {
	return &platform{pageKiB: pageKiB()}, nil
}

func (s *Sampler) collect(snap *Snapshot, ticks map[tickKey]uint64, elapsed float64) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := "/proc/" + e.Name()
		p, start, ok := s.readStat(dir+"/stat", pid)
		if !ok {
			continue
		}
		p.TGID = pid
		p.UID = dirUID(dir)
		p.User = s.userName(p.UID)
		p.Shr = s.readShared(dir + "/statm")
		p.Command = readCmdline(dir+"/cmdline", p.Comm)
		s.finish(&p, start, ticks, elapsed, snap.Mem.Total)
		snap.Procs = append(snap.Procs, p)

		if p.Kernel {
			snap.KernelThreads++
			snap.countRunning(p)
			continue
		}
		snap.Tasks++
		threads := s.collectThreads(dir, p, ticks, elapsed, snap.Mem.Total)
		if len(threads) == 0 {
			snap.countRunning(p)
		}
		for _, t := range threads {
			snap.countRunning(t)
		}
		snap.UserThreads += len(threads)
		snap.Procs = append(snap.Procs, threads...)
	}
}

func (s *Sampler) collectThreads(dir string, parent Process, ticks map[tickKey]uint64, elapsed float64, memTotal uint64) []Process {
	if parent.Threads <= 1 {
		return nil
	}
	entries, err := os.ReadDir(dir + "/task")
	if err != nil {
		return nil
	}
	threads := make([]Process, 0, len(entries))
	for _, e := range entries {
		tid, err := strconv.Atoi(e.Name())
		if err != nil || tid == parent.PID {
			continue
		}
		t, start, ok := s.readStat(dir+"/task/"+e.Name()+"/stat", tid)
		if !ok {
			continue
		}
		t.Thread = true
		t.TGID = parent.PID
		t.PPID = parent.PID
		t.UID, t.User = parent.UID, parent.User
		t.Shr = parent.Shr
		t.Command = parent.Command
		s.finish(&t, start, ticks, elapsed, memTotal)
		threads = append(threads, t)
	}
	return threads
}

func (s *Sampler) readStat(path string, id int) (Process, uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Process{}, 0, false
	}
	open := bytes.IndexByte(data, '(')
	closing := bytes.LastIndexByte(data, ')')
	if open < 0 || closing < open || closing+2 >= len(data) {
		return Process{}, 0, false
	}
	f := strings.Fields(string(data[closing+2:]))
	if len(f) < 22 {
		return Process{}, 0, false
	}
	num := func(i int) uint64 {
		v, _ := strconv.ParseUint(f[i], 10, 64)
		return v
	}
	signed := func(i int) int {
		v, _ := strconv.Atoi(f[i])
		return v
	}
	p := Process{
		PID:      id,
		PPID:     signed(1),
		Comm:     string(data[open+1 : closing]),
		State:    f[0][0],
		Kernel:   num(6)&kernelThreadFlag != 0,
		Ticks:    num(11) + num(12),
		Priority: signed(15),
		Nice:     signed(16),
		Threads:  signed(17),
		Virt:     num(20) / 1024,
		Res:      num(21) * s.plat.pageKiB,
	}
	return p, num(19), true
}

func (s *Sampler) readShared(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	f := strings.Fields(string(data))
	if len(f) < 3 {
		return 0
	}
	v, _ := strconv.ParseUint(f[2], 10, 64)
	return v * s.plat.pageKiB
}

func pageKiB() uint64 {
	if size := os.Getpagesize(); size > 0 {
		return uint64(size) / 1024
	}
	return 4
}

func dirUID(dir string) uint32 {
	info, err := os.Stat(dir)
	if err != nil {
		return 0
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Uid
	}
	return 0
}

func readCmdline(path, comm string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return comm
	}
	data = bytes.TrimRight(data, "\x00")
	for i, b := range data {
		if b == 0 || b == '\n' || b == '\t' {
			data[i] = ' '
		}
	}
	return string(data)
}
