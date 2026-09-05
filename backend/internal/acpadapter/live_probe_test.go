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

func TestLiveArchives(t *testing.T) {
	tests := []struct {
		agent          string
		envFiles       []string
		runtimeEnv     string
		runtimeVersion string
	}{
		{"codex", []string{"CODEX_PATH", "CODEX_CODE_MODE_HOST_PATH"}, "CODEX_PATH", "codex-cli 0.153.4"},
		{"claude", []string{"CLAUDE_CODE_EXECUTABLE"}, "CLAUDE_CODE_EXECUTABLE", "2.1.261 (Claude Code)"},
	}
	for _, test := range tests {
		t.Run(test.agent, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			manager := New(t.TempDir(), "dev")
			launch, err := manager.ResolveAdapter(ctx, test.agent)
			if err != nil {
				t.Fatal(err)
			}
			paths := []string{launch.Command}
			for _, key := range test.envFiles {
				paths = append(paths, launch.Env[key])
			}
			for _, path := range paths {
				if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
					t.Fatalf("managed runtime file %q: info=%v err=%v", path, info, err)
				}
			}
			version := manager.Status(test.agent).Version
			if output, err := exec.CommandContext(ctx, launch.Command, "--version").CombinedOutput(); err != nil ||
				version == "" || !strings.Contains(string(output), version) {
				t.Fatalf("adapter --version: %s err=%v", output, err)
			}
			if output, err := exec.CommandContext(ctx, launch.Env[test.runtimeEnv], "--version").CombinedOutput(); err != nil ||
				!strings.Contains(string(output), test.runtimeVersion) {
				t.Fatalf("runtime --version: %s err=%v", output, err)
			}
		})
	}
}
