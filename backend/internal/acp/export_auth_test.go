package acp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsolatedCloudLoginCapturesFreshCredentialsWithoutTouchingLocal(t *testing.T) {
	bin := t.TempDir()
	claudeBin := filepath.Join(bin, "claude")
	if err := os.WriteFile(claudeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	kimiBin := filepath.Join(bin, "kimi")
	if err := os.WriteFile(kimiBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
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
