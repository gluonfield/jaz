package acp_test

import (
	"syscall"
	"testing"
	"time"
)

// cpuTime is the CPU time the test process has used.
func cpuTime(t *testing.T) time.Duration {
	t.Helper()
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	ticks := func(ft syscall.Filetime) int64 {
		return int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	}
	return time.Duration((ticks(kernel) + ticks(user)) * 100)
}
