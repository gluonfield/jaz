//go:build !windows

package acp

import (
	"bufio"
	"os/exec"
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
	t.Cleanup(func() {
		_ = process.terminate()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}

	if err := process.terminate(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

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
