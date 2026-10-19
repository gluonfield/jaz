package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotWorktreeIsKeptUntilEveryOwnerIsArchivedAndStopped(t *testing.T) {
	workspace, _, worktree, store, worker := managedWorktreeSession(t, "bot-work", storage.RuntimeACP)
	worker.SourceType = storage.SourceBotWorker
	worker.SourceID = "bot"
	if err := store.SaveSession(worker); err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: store, Workspace: workspace}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("unarchived bot work was removed: %v", err)
	}
	if err := store.SetArchived(worker.ID, true); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(worktree, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "review.txt"), []byte("review evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	peer, err := store.CreateSession(storage.CreateSession{Slug: "review", ParentID: worker.ID, SourceType: storage.SourceBotWorker, SourceID: "bot", Runtime: storage.RuntimeACP, RuntimeRef: &storage.RuntimeRef{Cwd: nested, ProjectPath: nested}})
	if err != nil {
		t.Fatal(err)
	}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("another thread's shared worktree was removed: %v", err)
	}
	if err := store.SetArchived(peer.ID, true); err != nil {
		t.Fatal(err)
	}
	server.ACP = &fakeACPManager{jobs: []acp.Job{{ID: peer.ID, State: acp.StateIdle}}}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("a live idle process lost its directory: %v", err)
	}
	server.ACP = &fakeACPManager{}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("archived bot work was not removed: %v", err)
	}
	info, ok := server.missingWorktreeInfo(t.Context(), peer)
	if !ok || !info.WorktreeRestorable || info.Branch != "jaz/bot-work" {
		t.Fatalf("reviewer cannot restore shared worktree: %+v", info)
	}
	if err := server.ensureManagedWorktree(t.Context(), peer); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(content) != "two\n" {
		t.Fatalf("uncommitted work was not recoverable: %q, %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(nested, "review.txt")); err != nil || string(content) != "review evidence\n" {
		t.Fatalf("reviewer's directory was not recoverable: %q, %v", content, err)
	}
}

func TestUnarchivedBotWorkStaysOutsideTheOrdinaryRetentionCap(t *testing.T) {
	workspace, repo, _, store, _ := managedWorktreeSession(t, "ordinary", storage.RuntimeACP)
	var directories []string
	for i := range 17 {
		slug := fmt.Sprintf("bot-%d", i)
		cwd, _, err := gitinfo.AddWorktree(t.Context(), workspace, repo, slug, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.CreateSession(storage.CreateSession{Slug: slug, SourceType: storage.SourceBotWorker, RuntimeRef: &storage.RuntimeRef{Cwd: cwd, ProjectPath: repo}})
		if err != nil {
			t.Fatal(err)
		}
		directories = append(directories, cwd)
	}
	server := &Server{Store: store, Workspace: workspace}
	server.PruneManagedWorktrees(t.Context())
	for _, cwd := range directories {
		if _, err := os.Stat(cwd); err != nil {
			t.Fatalf("unarchived bot work entered the retention cap: %v", err)
		}
	}
}
