package acp

import "golang.org/x/sys/unix"

func memoryPressure() bool {
	level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	// The sysctl exports dispatch flags, not the kernel's internal enum.
	return err == nil && level&(2|4) != 0
}
