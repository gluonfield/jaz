package subprocess

import (
	"os/exec"
	"time"
)

const (
	// Give native agents time to persist rotated OAuth credentials before forcing exit.
	terminateGrace = 5 * time.Second
	stdioDrain     = 2 * time.Second
)

func New(cmd *exec.Cmd) *Supervisor {
	prepareCommand(cmd)
	process := &Supervisor{cmd: cmd}
	if cmd.Cancel != nil {
		cmd.Cancel = process.Terminate
	}
	cmd.WaitDelay = terminateGrace + stdioDrain
	return process
}
