//go:build !windows

package acp

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func prepareProcessCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

type processSupervisor struct {
	cmd *exec.Cmd
}

func newProcessSupervisor(cmd *exec.Cmd) *processSupervisor {
	return &processSupervisor{cmd: cmd}
}

func (p *processSupervisor) started() error {
	return nil
}

func (p *processSupervisor) terminate() error {
	if p.cmd.Process == nil {
		return nil
	}
	group := -p.cmd.Process.Pid
	if err := syscall.Kill(group, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}
	for deadline := time.Now().Add(processTerminateGrace); time.Now().Before(deadline); {
		if groupGone(group) {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(group, syscall.SIGKILL); err != nil && !groupSignalDone(err) {
		return err
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func groupGone(group int) bool {
	err := syscall.Kill(group, 0)
	return err != nil && groupSignalDone(err)
}

// A group whose last member has exited but not yet been reaped is unsignalable
// rather than absent, so EPERM means the same thing as ESRCH here: nothing is
// left to stop.
func groupSignalDone(err error) bool {
	return err == syscall.ESRCH || err == syscall.EPERM
}
