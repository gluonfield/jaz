//go:build !unix

package machinebridge

import "os/exec"

func configureExecProcessGroup(*exec.Cmd) {}

func terminateExecProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killExecProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
