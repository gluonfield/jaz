package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveManagedWorktreeUsesConfiguredIdentityAndSigning(t *testing.T) {
	dir := commitFixture(t)
	key := filepath.Join(t.TempDir(), "signing")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "fixture", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	public, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(t.TempDir(), "allowed_signers")
	if err := os.WriteFile(allowed, append([]byte("committer@example.invalid "), public...), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"author.name":                "Fixture Author",
		"author.email":               "author@example.invalid",
		"committer.name":             "Fixture Committer",
		"committer.email":            "committer@example.invalid",
		"gpg.format":                 "ssh",
		"user.signingkey":            key,
		"gpg.ssh.allowedSignersFile": allowed,
		"commit.gpgsign":             "true",
	} {
		commitGit(t, dir, "config", name, value)
	}
	workspace := t.TempDir()
	worktree, _, err := AddWorktree(t.Context(), workspace, dir, "configured", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CommitAll(t.Context(), worktree, "hook must run"); err == nil {
		t.Fatal("ordinary commits bypassed the user's hook")
	}
	branch := "jaz/configured"
	if err := RemoveManagedWorktree(t.Context(), worktree, branch, "snapshot"); err != nil {
		t.Fatal(err)
	}
	if got := commitGit(t, dir, "show", "-s", "--format=%an%n%ae%n%cn%n%ce", branch); got != "Fixture Author\nauthor@example.invalid\nFixture Committer\ncommitter@example.invalid" {
		t.Fatal("snapshot did not preserve configured author and committer")
	}
	commitGit(t, dir, "verify-commit", branch)
	if err := RestoreManagedWorktree(t.Context(), dir, worktree, branch); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "saved\n" {
		t.Fatalf("restored contents = %q, %v", got, err)
	}
}

func TestRemoveManagedWorktreePreservesWorkWhenCommitConfigurationFails(t *testing.T) {
	for _, failure := range []string{"identity", "signing"} {
		t.Run(failure, func(t *testing.T) {
			dir := commitFixture(t)
			if failure == "identity" {
				commitGit(t, dir, "config", "--unset", "user.name")
				commitGit(t, dir, "config", "--unset", "user.email")
			} else {
				commitGit(t, dir, "config", "gpg.format", "ssh")
				commitGit(t, dir, "config", "user.signingkey", filepath.Join(t.TempDir(), "missing-key"))
				commitGit(t, dir, "config", "commit.gpgsign", "true")
			}
			worktree, _, err := AddWorktree(t.Context(), t.TempDir(), dir, "failed", "")
			if err != nil {
				t.Fatal(err)
			}
			before := commitGit(t, worktree, "rev-parse", "HEAD")
			if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("keep me\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := RemoveManagedWorktree(t.Context(), worktree, "jaz/failed", "snapshot"); err == nil {
				t.Fatalf("snapshot silently bypassed configured %s", failure)
			}
			if got := commitGit(t, worktree, "rev-parse", "HEAD"); got != before {
				t.Fatal("failed snapshot changed HEAD")
			}
			if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "keep me\n" {
				t.Fatalf("failed snapshot lost working contents: %q, %v", got, err)
			}
			if got := commitGit(t, worktree, "show", ":file.txt"); got != "keep me" {
				t.Fatalf("failed snapshot lost staged contents: %q", got)
			}
		})
	}
}

func commitFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "absent"))
	dir := t.TempDir()
	commitGit(t, dir, "init", "-q", "-b", "main")
	for name, value := range map[string]string{
		"user.useConfigOnly": "true",
		"user.name":          "Fixture User",
		"user.email":         "fixture@example.invalid",
	} {
		commitGit(t, dir, "config", name, value)
	}
	commitGit(t, dir, "commit", "--allow-empty", "-qm", "initial")
	return dir
}

func commitGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(t.Context(), dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}
