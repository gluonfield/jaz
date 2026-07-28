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

	launch, err := New(t.TempDir(), "dev").ResolveAdapter(ctx, "codex")
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
	if output, err := exec.CommandContext(ctx, launch.Command, "--version").CombinedOutput(); err != nil ||
		!strings.Contains(string(output), "1.1.7-jaz.9") {
		t.Fatalf("adapter --version: %s err=%v", output, err)
	}
	if output, err := exec.CommandContext(ctx, launch.Env["CODEX_PATH"], "--version").CombinedOutput(); err != nil ||
		!strings.Contains(string(output), "codex-cli 0.145.0") {
		t.Fatalf("codex --version: %s err=%v", output, err)
	}
}
