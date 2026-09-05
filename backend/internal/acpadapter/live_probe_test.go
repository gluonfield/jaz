//go:build acpprobe

package acpadapter

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLiveCodexArchive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	manager := New(t.TempDir(), "dev")
	launch, err := manager.ResolveAdapter(ctx, "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		launch.Command,
		launch.Env["CODEX_PATH"],
		launch.Env["CODEX_CODE_MODE_HOST_PATH"],
	} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("managed runtime file %q: info=%v err=%v", path, info, err)
		}
	}
	version := manager.Status("codex").Version
	if output, err := exec.CommandContext(ctx, launch.Command, "--version").CombinedOutput(); err != nil ||
		version == "" || !strings.Contains(string(output), version) {
		t.Fatalf("adapter --version: %s err=%v", output, err)
	}
	if output, err := exec.CommandContext(ctx, launch.Env["CODEX_PATH"], "--version").CombinedOutput(); err != nil ||
		!strings.Contains(string(output), "codex-cli 0.153.4") {
		t.Fatalf("codex --version: %s err=%v", output, err)
	}
}
