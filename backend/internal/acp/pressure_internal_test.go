//go:build !windows

package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/gitinfo"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestPressureStopsOneOldestEligibleProcessAndResumesItsSession(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{Root: t.TempDir(), Workspace: t.TempDir(), Agents: map[string]AgentConfig{"fake": {
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeACPAgentProcess"},
		Env:     map[string]string{"JAZ_FAKE_ACP_AGENT": "1", "JAZ_FAKE_ACP_RESUME": "1"},
	}}}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	repo := filepath.Join(manager.cfg.Workspace, "repo")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q", "-b", "main", repo)
	git("-C", repo, "-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "--allow-empty", "-qm", "initial")
	var ids []string
	for i := range 4 {
		spawned, err := manager.Spawn(ctx, SpawnRequest{ACPAgent: "fake", Slug: fmt.Sprintf("pressure-%d", i), Directory: repo, Worktree: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, spawned.SessionID)
	}
	removed := false
	if err := manager.WithStoppedSessions(ids, func() error {
		removed = true
		return nil
	}); err != nil || removed {
		t.Fatalf("worktree cleanup reached live processes: %v", err)
	}
	now := time.Now()
	for i, age := range []time.Duration{5 * time.Minute, 4 * time.Minute, 2 * time.Minute, 30 * time.Second} {
		manager.processes[ids[i]].conn.lastTraffic.Store(now.Add(-age).UnixNano())
	}
	held := manager.jobByID(ids[0])
	if !held.lease() {
		t.Fatal("cannot hold the oldest process")
	}
	manager.stopIdle(now, DefaultIdleTimeouts, false)
	for _, id := range ids {
		if manager.jobByID(id) == nil {
			t.Fatal("stopped before the normal timeout without pressure")
		}
	}
	original := manager.jobByID(ids[1]).Snapshot().ACPSession
	cwd := manager.jobByID(ids[1]).Snapshot().Cwd
	manager.stopIdle(now, DefaultIdleTimeouts, true)
	for i, id := range ids {
		if stopped := manager.jobByID(id) == nil; stopped != (i == 1) {
			t.Fatalf("process %d stopped = %v, want only the oldest unleased process", i, stopped)
		}
	}
	held.release()
	manager.stopIdle(now, DefaultIdleTimeouts, true)
	if manager.jobByID(ids[0]) != nil || manager.jobByID(ids[2]) == nil || manager.jobByID(ids[3]) == nil {
		t.Fatal("pressure did not reclaim exactly the next oldest eligible process")
	}
	if err := manager.WithStoppedSessions([]string{ids[1]}, func() error {
		return gitinfo.RemoveManagedWorktree(ctx, cwd, "jaz/pressure-1", "snapshot")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatalf("stopped thread's worktree was not removed: %v", err)
	}
	job, err := manager.resume(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if job.Snapshot().ACPSession != original {
		t.Fatal("pressure stop replaced the native conversation")
	}
	if _, err := os.Stat(cwd); err != nil {
		t.Fatalf("resume did not restore the worktree: %v", err)
	}
	manager.stopIdle(now, DefaultIdleTimeouts, false)
	if manager.jobByID(ids[2]) == nil {
		t.Fatal("continued early reclamation after pressure cleared")
	}
	manager.stopIdle(now.Add(9*time.Minute), DefaultIdleTimeouts, true)
	if manager.jobByID(ids[2]) != nil {
		t.Fatal("pressure rate limit prevented normal timeout cleanup")
	}
}
