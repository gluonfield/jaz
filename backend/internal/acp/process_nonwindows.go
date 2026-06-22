//go:build !windows

package acp

import "os/exec"

func hideProcessWindow(cmd *exec.Cmd) {}
