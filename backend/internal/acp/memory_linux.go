package acp

import "os"

func memoryPressure() bool {
	data, err := os.ReadFile("/proc/meminfo")
	return err == nil && lowAvailableMemory(string(data))
}
