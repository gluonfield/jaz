package acp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestResolveLoginExecutablePrefersBundle(t *testing.T) {
	dir := t.TempDir()
	want := writeExecutable(t, dir, "jaz-fake-login-cli")
	got, err := resolveLoginExecutable(dir, "jaz-fake-login-cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("expected bundled path %q, got %q", want, got)
	}
}

func TestResolveLoginExecutableMissingEverywhere(t *testing.T) {
	// Empty bundle dir and a name that cannot exist on PATH: must error so the
	// onboarding probe reports the binary as not found instead of pretending.
	if _, err := resolveLoginExecutable(t.TempDir(), "jaz-definitely-not-installed-xyz"); err == nil {
		t.Fatal("expected not-found error when the login CLI is absent from bundle and PATH")
	}
}

func TestAgentLoginInvocationForUsesBundledClaude(t *testing.T) {
	// The claude adapter bundles its CLI; login must resolve there even when
	// claude is not on PATH, so a Node-free backend can sign in.
	bundle := t.TempDir()
	want := writeExecutable(t, bundle, "claude")
	inv := AgentLoginInvocationFor(AgentClaude, t.TempDir(), AgentAuthConfig{}, bundle)
	if !inv.Available {
		t.Fatalf("expected bundled claude login to be available, reason=%q", inv.Reason)
	}
	if inv.Executable != want {
		t.Fatalf("expected executable %q, got %q", want, inv.Executable)
	}
	if !strings.Contains(inv.Display, want) {
		t.Fatalf("expected display to reference the bundled binary, got %q", inv.Display)
	}
}

func TestAgentLoginInvocationForCodexBundleWithoutLoginCLI(t *testing.T) {
	// The codex adapter bundles codex-acp but not the codex login CLI. With an
	// empty-of-codex bundle and codex absent from PATH, login must be reported
	// unavailable with a reason rather than silently hidden.
	bundle := t.TempDir()
	writeExecutable(t, bundle, "codex-acp")
	if _, err := ResolveExecutable("codex"); err == nil {
		t.Skip("codex is installed on PATH in this environment")
	}
	inv := AgentLoginInvocationFor(AgentCodex, t.TempDir(), AgentAuthConfig{}, bundle)
	if inv.Available {
		t.Fatal("expected codex login to be unavailable without a bundled or PATH codex CLI")
	}
	if strings.TrimSpace(inv.Reason) == "" {
		t.Fatal("expected a not-found reason for the missing codex login CLI")
	}
}
