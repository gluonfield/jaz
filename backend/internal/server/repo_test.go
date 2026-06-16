package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionRepoChangesAndDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "diff-session",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeNative, Cwd: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	noCwd, err := store.CreateSession(storage.CreateSession{Slug: "no-cwd"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: store}

	getJSON := func(path string, want int, into any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("GET %s = %d, want %d; body = %s", path, res.Code, want, res.Body.String())
		}
		if into != nil {
			if err := json.Unmarshal(res.Body.Bytes(), into); err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
		}
	}

	var changes gitinfo.ChangeSummary
	getJSON("/v1/sessions/"+session.ID+"/repo/changes", http.StatusOK, &changes)
	if len(changes.Files) != 1 || changes.Files[0].Path != "file.txt" || changes.Files[0].Status != "modified" {
		t.Fatalf("changes = %+v, want the dirty file.txt", changes.Files)
	}
	if changes.TotalAdded != 1 || changes.TotalDeleted != 1 {
		t.Errorf("totals = +%d -%d, want +1 -1", changes.TotalAdded, changes.TotalDeleted)
	}
	base := changes.Base
	if base == "" {
		t.Fatal("changes summary did not report its base")
	}

	// No cwd degrades to an empty summary rather than an error.
	getJSON("/v1/sessions/"+noCwd.ID+"/repo/changes", http.StatusOK, &changes)
	if len(changes.Files) != 0 {
		t.Errorf("no-cwd changes = %+v, want empty", changes.Files)
	}

	var patch gitinfo.FilePatch
	getJSON("/v1/sessions/"+session.ID+"/repo/diff?path=file.txt&base="+base, http.StatusOK, &patch)
	if patch.Path != "file.txt" || patch.Binary || patch.Truncated {
		t.Fatalf("patch = %+v, want plain file.txt diff", patch)
	}
	for _, want := range []string{"@@", "-old", "+new"} {
		if !strings.Contains(patch.Patch, want) {
			t.Errorf("patch missing %q:\n%s", want, patch.Patch)
		}
	}

	// The untracked flavor diffs against /dev/null.
	if err := os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	getJSON("/v1/sessions/"+session.ID+"/repo/diff?path=extra.txt&untracked=1", http.StatusOK, &patch)
	if !strings.Contains(patch.Patch, "+extra") {
		t.Errorf("untracked patch = %+v, want +extra", patch)
	}

	// Paths reach `git diff --`; anything escaping the cwd is rejected.
	for _, bad := range []string{"", "../secret", "/etc/passwd", "ok/../../up"} {
		getJSON("/v1/sessions/"+session.ID+"/repo/diff?path="+url.QueryEscape(bad), http.StatusBadRequest, nil)
	}
	getJSON("/v1/sessions/"+session.ID+"/repo/diff?path=file.txt&old_path="+url.QueryEscape("../secret"), http.StatusBadRequest, nil)
	// base reaches git argv; only commit hashes pass.
	for _, bad := range []string{"..", "HEAD", "-Oorigin", "zzzz"} {
		getJSON("/v1/sessions/"+session.ID+"/repo/diff?path=file.txt&base="+url.QueryEscape(bad), http.StatusBadRequest, nil)
	}
	// Diffs need a working directory, unlike the degrading changes summary.
	getJSON("/v1/sessions/"+noCwd.ID+"/repo/diff?path=file.txt", http.StatusBadRequest, nil)
}

func TestArchivePrunesAndRestoresManagedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace, _, worktree, store, session := managedWorktreeSession(t, "managed-thread", storage.RuntimeNative)
	server := &Server{Store: store, Workspace: workspace}
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/archive", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body = %s", res.Code, res.Body.String())
	}
	waitFor(t, 2*time.Second, func() bool {
		_, err := os.Stat(worktree)
		return os.IsNotExist(err)
	})

	var info gitinfo.Info
	req = httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/repo", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("repo status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.WorktreeMissing || !info.WorktreeRestorable || info.WorktreeBranch != "jaz/managed-thread" {
		t.Fatalf("repo info = %+v, want restorable missing worktree", info)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/repo/restore-worktree", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.Git || info.Branch != "jaz/managed-thread" || !info.IsWorktree {
		t.Fatalf("restored repo info = %+v", info)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "two\n" {
		t.Fatalf("restored file.txt = %q, %v", got, err)
	}
}

func TestBeginACPTurnRestoresMissingManagedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := t.Context()
	workspace, repo, worktree, store, session := managedWorktreeSession(t, "resume-thread", storage.RuntimeACP)
	if err := gitinfo.RemoveManagedWorktree(ctx, worktree, "jaz/resume-thread", "snapshot"); err != nil {
		t.Fatalf("RemoveManagedWorktree: %v", err)
	}
	server := &Server{Store: store, Workspace: workspace}
	if _, err := server.beginACPTurn(ctx, session, "continue"); err != nil {
		t.Fatalf("beginACPTurn: %v", err)
	}
	if info := gitinfo.Inspect(ctx, worktree); !info.Git || info.Branch != "jaz/resume-thread" || !info.IsWorktree {
		t.Fatalf("restored repo info = %+v", info)
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "file.txt")); err != nil || string(got) != "two\n" {
		t.Fatalf("restored file.txt = %q, %v", got, err)
	}
	if !gitinfo.BranchExists(ctx, repo, "jaz/resume-thread") {
		t.Fatal("restored branch missing")
	}
}

func TestPruneSkipsLiveACPManagedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace, _, worktree, store, session := managedWorktreeSession(t, "live-acp-thread", storage.RuntimeACP)
	if err := store.SetArchived(session.ID, true); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Store:     store,
		Workspace: workspace,
		ACP: &fakeACPManager{jobs: []acp.Job{{
			ID:    session.ID,
			Slug:  session.Slug,
			State: acp.StateIdle,
		}}},
	}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("live ACP worktree was pruned: %v", err)
	}

	server.ACP = &fakeACPManager{}
	server.PruneManagedWorktrees(t.Context())
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("inactive ACP worktree still exists after prune: %v", err)
	}
}

func managedWorktreeSession(t *testing.T, slug, runtime string) (workspace, repo, worktree string, store *jsonstore.Store, session storage.Session) {
	t.Helper()
	ctx := t.Context()
	workspace = t.TempDir()
	repo = filepath.Join(workspace, "repo")
	git := func(target string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", target, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.Command("git", "init", "-q", "-b", "main", repo).Run(); err != nil {
		t.Fatal(err)
	}
	git(repo, "config", "user.email", "t@t")
	git(repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", "-A")
	git(repo, "commit", "-q", "-m", "init")
	var err error
	worktree, repo, err = gitinfo.AddWorktree(ctx, workspace, repo, slug, "")
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "file.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err = jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err = store.CreateSession(storage.CreateSession{
		Slug:       slug,
		Title:      "managed thread",
		Runtime:    runtime,
		RuntimeRef: &storage.RuntimeRef{Type: runtime, Cwd: worktree, ProjectPath: repo},
	})
	if err != nil {
		t.Fatal(err)
	}
	return workspace, repo, worktree, store, session
}
