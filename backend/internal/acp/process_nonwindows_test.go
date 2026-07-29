//go:build !windows

package acp

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPrepareProcessCommandStartsOwnProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60")

	prepareProcessCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Fatal("Setpgid = false")
	}
}

func TestProcessSupervisorAcceptsExitedProcess(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	prepareProcessCommand(cmd)
	process := newProcessSupervisor(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if err := process.terminate(); err != nil {
		t.Fatalf("process.terminate after exit = %v", err)
	}
}

// Claude Code persists a rotated OAuth refresh token on its way out, so the
// supervisor must ask before it forces; SIGKILL mid-rotation is what strands a
// credential the provider has already retired.
func TestProcessSupervisorSignalsTerminationBeforeKilling(t *testing.T) {
	state := runSupervisedShell(t, `trap 'exit 0' TERM`)

	if code := state.ExitCode(); code != 0 {
		t.Fatalf("exit code = %d (%v), want the shutdown handler to run and exit cleanly", code, state)
	}
}

func TestProcessSupervisorKillsProcessThatIgnoresTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the full termination grace")
	}
	state := runSupervisedShell(t, `trap '' TERM`)

	status, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("exit = %v, want a process that ignores termination to be killed", state)
	}
}

// runSupervisedShell starts a shell under the same wiring openConn uses, waits
// for it to install the given signal disposition, cancels its context, and
// reports how the shell ended.
func runSupervisedShell(t *testing.T, disposition string) *os.ProcessState {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.CommandContext(ctx, "sh", "-c", disposition+`; : > "$0"; while :; do sleep 0.05; done`, ready)
	prepareProcessCommand(cmd)
	process := newProcessSupervisor(cmd)
	cmd.Cancel = process.terminate
	cmd.WaitDelay = processTerminateGrace + acpProcessStdioDrain
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shell never installed its signal disposition")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-waitErr
	return cmd.ProcessState
}

func TestProcessSupervisorKillsProcessGroupChildren(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60 & echo $!; wait")
	prepareProcessCommand(cmd)
	process := newProcessSupervisor(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = process.terminate()
		<-waitErr
	})

	if err := process.terminate(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(childPID, 0)
		if err == syscall.ESRCH {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d still exists after process-group termination: %v", childPID, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
