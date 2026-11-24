package acp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsolatedCloudLoginCapturesFreshCredentialsWithoutTouchingLocal(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"claude", "kimi"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root := t.TempDir()
	claudeLogin, err := PrepareIsolatedCloudLogin(AgentClaude, AgentConfig{LoginBinDir: bin}, root)
	if err != nil {
		t.Fatalf("PrepareIsolatedCloudLogin(claude): %v", err)
	}
	defer claudeLogin.Cleanup()
	if claudeLogin.Invocation.Env["CLAUDE_CONFIG_DIR"] != claudeLogin.Dir {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want isolated %q", claudeLogin.Invocation.Env["CLAUDE_CONFIG_DIR"], claudeLogin.Dir)
	}

	creds := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-fresh","refreshToken":"rt-fresh","expiresAt":1893456000000}}`
	meta := `{"oauthAccount":{"emailAddress":"dev@example.com"}}`
	if err := os.WriteFile(filepath.Join(claudeLogin.Dir, ".credentials.json"), []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeLogin.Dir, ".claude.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	gotClaude, err := CaptureIsolatedAuthFiles(AgentClaude, claudeLogin.Dir)
	if err != nil {
		t.Fatalf("CaptureIsolatedAuthFiles(claude): %v", err)
	}
	if string(gotClaude[".credentials.json"]) != creds || string(gotClaude[".claude.json"]) != meta {
		t.Fatalf("unexpected claude capture: %+v", gotClaude)
	}

	kimiLogin, err := PrepareIsolatedCloudLogin(AgentKimi, AgentConfig{LoginBinDir: bin}, root)
	if err != nil {
		t.Fatalf("PrepareIsolatedCloudLogin(kimi): %v", err)
	}
	defer kimiLogin.Cleanup()
	if err := os.MkdirAll(filepath.Join(kimiLogin.Dir, "credentials"), 0o700); err != nil {
		t.Fatal(err)
	}
	kimiTok := `{"access_token":"kimi-fresh"}`
	if err := os.WriteFile(filepath.Join(kimiLogin.Dir, "credentials", "kimi-code.json"), []byte(kimiTok), 0o600); err != nil {
		t.Fatal(err)
	}
	gotKimi, err := CaptureIsolatedAuthFiles(AgentKimi, kimiLogin.Dir)
	if err != nil {
		t.Fatalf("CaptureIsolatedAuthFiles(kimi): %v", err)
	}
	if string(gotKimi["credentials/kimi-code.json"]) != kimiTok {
		t.Fatalf("unexpected kimi capture: %+v", gotKimi)
	}

	if _, err := PrepareIsolatedCloudLogin(AgentCodex, AgentConfig{}, root); !errors.Is(err, ErrUnsupportedCloudLoginAgent) {
		t.Fatalf("expected ErrUnsupportedCloudLoginAgent for codex, got %v", err)
	}
}

func TestCloudLoginRunnerCancelsPreviousAttemptForSameAgent(t *testing.T) {
	slowBin := filepath.Join(t.TempDir(), "slow-claude")
	if err := os.WriteFile(slowBin, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fastBin := filepath.Join(t.TempDir(), "fast-claude")
	fastScript := `#!/bin/sh
echo "Visit https://claude.ai/oauth/test"
printf '{"claudeAiOauth":{"accessToken":"sk-ant-oat01-second","refreshToken":"rt-2","expiresAt":1893456000000}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
`
	if err := os.WriteFile(fastBin, []byte(fastScript), 0o755); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	runner := NewCloudLoginRunner(func(agent string) (IsolatedCloudLogin, error) {
		dir, err := os.MkdirTemp("", "jaz-runner-test-*")
		if err != nil {
			return IsolatedCloudLogin{}, err
		}
		bin := slowBin
		if calls.Add(1) > 1 {
			bin = fastBin
		}
		return IsolatedCloudLogin{
			Agent: agent,
			Dir:   dir,
			Invocation: AgentLoginInvocation{
				Executable: bin,
				Env:        map[string]string{"CLAUDE_CONFIG_DIR": dir},
				Available:  true,
			},
			Cleanup: func() { _ = os.RemoveAll(dir) },
		}, nil
	})

	first, err := runner.Start(AgentClaude, func(context.Context, string, map[string][]byte) error { return nil })
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	second, err := runner.Start(AgentClaude, func(context.Context, string, map[string][]byte) error { return nil })
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s1, _ := runner.Get(AgentClaude, first.ID)
		s2, _ := runner.Get(AgentClaude, second.ID)
		if s1.Status == "failed" && s2.Status == "completed" {
			if !strings.Contains(s1.Error, "replaced") {
				t.Fatalf("expected first login error to explain replacement, got %q", s1.Error)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	s1, _ := runner.Get(AgentClaude, first.ID)
	s2, _ := runner.Get(AgentClaude, second.ID)
	t.Fatalf("expected first=failed and second=completed, got s1=%+v s2=%+v", s1, s2)
}
