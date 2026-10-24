package managedtool

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

func TestMuseSetupUsesNativeLoginSelection(t *testing.T) {
	for _, selection := range []string{"host override", "agent override", "agent PATH", "install directory", "native default", "managed"} {
		t.Run(selection, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("LOCALAPPDATA", home)
			t.Setenv("PATH", t.TempDir())
			t.Setenv("MUSE_CLI", "")
			t.Setenv("MUSE_INSTALL_DIR", "")
			managed := ExecutablePath(root, MuseCLI)
			selected := filepath.Join(t.TempDir(), ExecutableName(MuseCLI))
			env := map[string]string{}
			switch selection {
			case "host override":
				t.Setenv("MUSE_CLI", selected)
			case "agent override":
				t.Setenv("MUSE_CLI", filepath.Join(root, "wrong-host-selection"))
				env["MUSE_CLI"] = selected
			case "agent PATH":
				env["PATH"] = filepath.Dir(selected)
			case "install directory":
				env["MUSE_INSTALL_DIR"] = filepath.Dir(selected)
			case "native default":
				selected = filepath.Join(home, ".local", "bin", ExecutableName(MuseCLI))
				if runtime.GOOS == "windows" {
					selected = filepath.Join(home, "Programs", "muse", ExecutableName(MuseCLI))
				}
			case "managed":
				selected = managed
			}
			if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(selected, []byte("native launcher"), 0o755); err != nil {
				t.Fatal(err)
			}
			if selection == "host override" || selection == "agent override" {
				if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(managed, []byte("other launcher"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			manager := New(root)
			manager.museInstallerURL = "http://127.0.0.1:1"
			if err := manager.Prepare(t.Context(), MuseCLI, env); err != nil {
				t.Fatalf("existing native selection required an installation: %v", err)
			}
			status := manager.Status(MuseCLI, env)
			login := acp.AgentLoginInvocationFor(acp.AgentMuse, root, acp.AgentAuthConfig{}, filepath.Dir(status.Path), env)
			if status.State != StateReady || status.Path != selected || !login.Available || login.Env["MUSE_CLI"] != selected {
				t.Fatalf("setup = %#v, login selection = %q, available = %v; want %s", status, login.Env["MUSE_CLI"], login.Available, selected)
			}
			if err := os.Remove(selected); err != nil {
				t.Fatal(err)
			}
			if status := manager.Status(MuseCLI, env); status.State == StateReady {
				t.Fatalf("removed native selection remained ready: %#v", status)
			}
		})
	}
}
