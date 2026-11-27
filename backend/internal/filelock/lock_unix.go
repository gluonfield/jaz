//go:build !windows

package filelock

import (
	"os"
	"syscall"
)

func lock(file *os.File, wait bool) error {
	flags := syscall.LOCK_EX
	if !wait {
		flags |= syscall.LOCK_NB
	}
	return syscall.Flock(int(file.Fd()), flags)
}

func unlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
