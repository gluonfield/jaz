package acp

import (
	"os/exec"
	"testing"
)

func TestHideProcessWindowSuppressesWindowsConsole(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", "echo ok")

	hideProcessWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CreationFlags = %#x, missing CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
}
