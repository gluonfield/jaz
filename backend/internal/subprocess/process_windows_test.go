package subprocess

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPrepareProcessCommandSuppressesWindowsConsole(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", "echo ok")

	prepareCommand(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, missing CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}

func TestProcessSupervisorTerminatesWindowsJobChildren(t *testing.T) {
	if os.Getenv("JAZ_ACP_WINDOWS_JOB_HELPER") == "1" {
		runWindowsJobChildHelper()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestProcessSupervisorTerminatesWindowsJobChildren")
	cmd.Env = append(os.Environ(), "JAZ_ACP_WINDOWS_JOB_HELPER=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	process := New(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := process.Started(); err != nil {
		_ = process.Terminate()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = process.Terminate()
		_ = cmd.Wait()
	})
	if _, err := stdin.Write([]byte{'\n'}); err != nil {
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

	if err := process.Terminate(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waitWindowsProcessGone(t, uint32(childPID))
}

func runWindowsJobChildHelper() {
	if _, err := bufio.NewReader(os.Stdin).ReadByte(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	child := exec.Command("cmd.exe", "/d", "/s", "/c", "ping -n 60 127.0.0.1 >nul")
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(child.Process.Pid)
	_ = child.Wait()
}

func waitWindowsProcessGone(t *testing.T, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				return
			}
			t.Fatalf("open child process %d: %v", pid, err)
		}
		_ = windows.CloseHandle(handle)
		if time.Now().After(deadline) {
			t.Fatalf("child process %d still exists after job termination", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
