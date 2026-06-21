package shellcmd

import "testing"

func TestCommandUsesWindowsShellFlags(t *testing.T) {
	shell, args := commandFor("windows", "cmd.exe", "dir", true)
	if shell != "cmd.exe" || len(args) != 4 || args[0] != "/d" || args[1] != "/s" || args[2] != "/c" || args[3] != "dir" {
		t.Fatalf("Command = %q %#v, want cmd-style invocation", shell, args)
	}
}

func TestCommandKeepsExplicitPOSIXShellFlags(t *testing.T) {
	shell, args := commandFor("windows", "bash", "echo ok", true)
	if shell != "bash" || len(args) != 2 || args[0] != "-lc" || args[1] != "echo ok" {
		t.Fatalf("Command = %q %#v, want bash -lc", shell, args)
	}
}
