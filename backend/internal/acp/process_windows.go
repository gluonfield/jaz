package acp

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareProcessCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

type processSupervisor struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	job windows.Handle
}

func newProcessSupervisor(cmd *exec.Cmd) *processSupervisor {
	return &processSupervisor{cmd: cmd}
}

func (p *processSupervisor) started() error {
	job, err := createACPProcessJob()
	if err != nil {
		return err
	}
	var assignErr error
	err = p.cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	})
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	if assignErr != nil {
		_ = windows.CloseHandle(job)
		return assignErr
	}

	p.mu.Lock()
	if p.job != 0 {
		_ = windows.CloseHandle(job)
	} else {
		p.job = job
	}
	p.mu.Unlock()
	return nil
}

func (p *processSupervisor) terminate() error {
	p.mu.Lock()
	job := p.job
	p.job = 0
	p.mu.Unlock()

	if job != 0 {
		return windows.CloseHandle(job)
	}
	if p.cmd.Process == nil {
		return nil
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func createACPProcessJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}
