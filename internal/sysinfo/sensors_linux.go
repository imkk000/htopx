package sysinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const hwmonRoot = "/sys/class/hwmon"

var cpuTempLabels = map[string][]string{
	"k10temp":     {"Tdie", "Tctl"},
	"zenpower":    {"Tdie", "Tctl"},
	"coretemp":    {"Package id 0", "Package id 1"},
	"cpu_thermal": {""},
}

var ramTempChips = map[string]bool{
	"spd5118": true,
	"jc42":    true,
}

var defaultCPUFan = map[string]string{
	"nct6": "fan2",
	"it87": "fan1",
	"it86": "fan1",
}

func (*platform) sensors() Sensors {
	var s Sensors
	entries, err := os.ReadDir(hwmonRoot)
	if err != nil {
		return s
	}
	for _, e := range entries {
		dir := filepath.Join(hwmonRoot, e.Name())
		name := readSysString(filepath.Join(dir, "name"))
		if labels, ok := cpuTempLabels[name]; ok {
			if t, ok := labeledTemp(dir, labels); ok && (!s.HasCPUTemp || t > s.CPUTemp) {
				s.CPUTemp, s.HasCPUTemp = t, true
			}
		}
		if ramTempChips[name] {
			if t, ok := readMilli(filepath.Join(dir, "temp1_input")); ok && (!s.HasRAMTemp || t > s.RAMTemp) {
				s.RAMTemp, s.HasRAMTemp = t, true
			}
		}
		if !s.HasCPUFan {
			s.CPUFanRPM, s.HasCPUFan = cpuFan(dir, name)
		}
	}
	return s
}

func labeledTemp(dir string, labels []string) (float64, bool) {
	inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
	byLabel := make(map[string]string, len(inputs))
	for _, in := range inputs {
		label := readSysString(strings.TrimSuffix(in, "_input") + "_label")
		if _, seen := byLabel[label]; !seen {
			byLabel[label] = in
		}
	}
	for _, label := range labels {
		if in, ok := byLabel[label]; ok {
			return readMilli(in)
		}
	}
	return 0, false
}

func cpuFan(dir, chip string) (int, bool) {
	inputs, _ := filepath.Glob(filepath.Join(dir, "fan*_input"))
	for _, in := range inputs {
		label := strings.ToLower(readSysString(strings.TrimSuffix(in, "_input") + "_label"))
		if strings.Contains(label, "cpu") || strings.Contains(label, "processor") {
			return readSysInt(in)
		}
	}
	for prefix, fan := range defaultCPUFan {
		if strings.HasPrefix(chip, prefix) {
			return readSysInt(filepath.Join(dir, fan+"_input"))
		}
	}
	return 0, false
}

func readSysString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysInt(path string) (int, bool) {
	v, err := strconv.Atoi(readSysString(path))
	return v, err == nil
}

func readMilli(path string) (float64, bool) {
	v, ok := readSysInt(path)
	return float64(v) / 1000, ok
}
