package gpu

import "time"

type Kind int

const (
	KindNone Kind = iota
	KindNVIDIA
	KindApple
)

type Device struct {
	Index       int
	Name        string
	BusID       string
	Util        uint32
	MemUtil     uint32
	MemUsed     uint64
	MemTotal    uint64
	Temp        uint32
	Fan         uint32
	HasFan      bool
	PowerW      float64
	PowerCapW   float64
	PState      int
	ClockSM     uint32
	ClockMem    uint32
	ClockSMMax  uint32
	ClockMemMax uint32
	EncUtil     uint32
	DecUtil     uint32
	Cores       int
	Renderer    uint32
	Tiler       uint32
	MemAlloc    uint64
}

type ProcUsage struct {
	GPU     int
	Type    string
	MemUsed uint64
	SM      uint32
	Mem     uint32
	Enc     uint32
	Dec     uint32
	GPUTime time.Duration
}

type Snapshot struct {
	Kind    Kind
	Driver  string
	CUDA    string
	Devices []Device
	Procs   map[int]ProcUsage
}
