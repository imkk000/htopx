package sysinfo

type Sensors struct {
	CPUTemp, RAMTemp                  float64
	CPUFanRPM                         int
	HasCPUTemp, HasRAMTemp, HasCPUFan bool
}
