//go:build !windows

package acp

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
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
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
