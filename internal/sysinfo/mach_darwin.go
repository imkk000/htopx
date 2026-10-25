package sysinfo

import (
	"fmt"

	"github.com/ebitengine/purego"
)

const (
	processorCPULoadInfo = 2
	hostVMInfo64         = 4
	hostVMInfo64Count    = 38
	procPIDTaskInfo      = 4
)

type procTaskInfo struct {
	VirtualSize      uint64
	ResidentSize     uint64
	TotalUser        uint64
	TotalSystem      uint64
	ThreadsUser      uint64
	ThreadsSystem    uint64
	Policy           int32
	Faults           int32
	Pageins          int32
	CowFaults        int32
	MessagesSent     int32
	MessagesReceived int32
	SyscallsMach     int32
	SyscallsUnix     int32
	Csw              int32
	ThreadNum        int32
	NumRunning       int32
	Priority         int32
}

type timebaseInfo struct {
	Numer, Denom uint32
}

type machLib struct {
	host              uint32
	task              uint32
	hostProcessorInfo func(host uint32, flavor int32, count *uint32, info **uint32, infoCount *uint32) int32
	hostStatistics64  func(host uint32, flavor int32, info *uint32, count *uint32) int32
	vmDeallocate      func(task uint32, addr, size uintptr) int32
	procPidInfo       func(pid, flavor int32, arg uint64, buf *procTaskInfo, size int32) int32
	timebase          timebaseInfo
}

func loadMach() (lib machLib, err error) {
	handle, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return lib, fmt.Errorf("load libSystem: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bind libSystem: %v", r)
		}
	}()
	var hostSelf, taskSelf func() uint32
	var timebase func(info *timebaseInfo) int32
	purego.RegisterLibFunc(&hostSelf, handle, "mach_host_self")
	purego.RegisterLibFunc(&taskSelf, handle, "mach_task_self")
	purego.RegisterLibFunc(&timebase, handle, "mach_timebase_info")
	purego.RegisterLibFunc(&lib.hostProcessorInfo, handle, "host_processor_info")
	purego.RegisterLibFunc(&lib.hostStatistics64, handle, "host_statistics64")
	purego.RegisterLibFunc(&lib.vmDeallocate, handle, "vm_deallocate")
	purego.RegisterLibFunc(&lib.procPidInfo, handle, "proc_pidinfo")
	lib.host, lib.task = hostSelf(), taskSelf()
	if timebase(&lib.timebase) != 0 || lib.timebase.Denom == 0 {
		lib.timebase = timebaseInfo{Numer: 1, Denom: 1}
	}
	return lib, nil
}
