package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestSessionMergeSelectedWorktreeBranch(t *testing.T) {
	workspace, repo, parent, store, session := managedWorktreeSession(t, "parent", "acp")
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(repo, "checkout", "-qb", "integration")
	child, _, err := gitinfo.AddWorktree(t.Context(), workspace, parent, "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "worker.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(child, "add", "worker.txt")
	run(child, "commit", "-qm", "worker changes")
	run(repo, "-c", "tag.gpgsign=false", "tag", "jaz/worker", "integration")
	run(repo, "branch", "-f", "main", "refs/heads/jaz/worker")
	run(repo, "branch", "unlinked", "refs/heads/jaz/worker")
	if err := os.WriteFile(filepath.Join(child, "draft.txt"), []byte("keep uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, Workspace: workspace}).Handler()
	request := func(method, action, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(method, "/v1/sessions/"+session.ID+"/repo"+action, strings.NewReader(body)))
		if res.Code != want {
			t.Fatalf("%s %s = %d, want %d: %s", method, action, res.Code, want, res.Body.String())
		}
		return res
	}
	var info gitinfo.Info
	if err := json.Unmarshal(request(http.MethodGet, "", "", http.StatusOK).Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Ahead == nil || *info.Ahead != 0 || info.MainBranch != "integration" {
		t.Fatalf("parent status = %+v, want no own commits into integration", info)
	}
	var view gitinfo.MergeBranchesView
	loadBranches := func() {
		t.Helper()
		if err := json.Unmarshal(request(http.MethodGet, "/merge-branches", "", http.StatusOK).Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
	}
	selection := func(source gitinfo.Revision, target string) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"source": source, "target": target})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	loadBranches()
	if view.Target != "integration" || len(view.Branches) != 1 || view.Branches[0].Branch != "jaz/worker" || view.Branches[0].Ahead != 1 {
		t.Fatalf("branches = %+v, want worker commit against integration", view)
	}
	source := view.Branches[0].Revision
	body := selection(source, view.Target)
	for _, invalid := range []string{"null", `{}`, `{"source":null,"target":"integration"}`, `{"source":42}`, body + " {}", body[:len(body)-1] + `,"unknown":1}`} {
		request(http.MethodPost, "/merge-branches", invalid, http.StatusBadRequest)
	}
	for _, branch := range []string{"unlinked", "integration", "--all", ""} {
		request(http.MethodPost, "/merge-branches", selection(gitinfo.Revision{Branch: branch, Head: source.Head}, view.Target), http.StatusBadRequest)
	}
	request(http.MethodPost, "/merge", `{"branch":null}`, http.StatusBadRequest)
	before := gitinfo.CurrentRevision(t.Context(), repo)
	run(child, "commit", "--allow-empty", "-qm", "new unreviewed work")
	request(http.MethodPost, "/merge-branches", body, http.StatusBadRequest)
	if after := gitinfo.CurrentRevision(t.Context(), repo); after != before {
		t.Fatalf("stale source changed destination: %+v -> %+v", before, after)
	}
	loadBranches()
	body = selection(view.Branches[0].Revision, view.Target)
	run(repo, "checkout", "-qb", "other-target")
	request(http.MethodPost, "/merge-branches", body, http.StatusBadRequest)
	if after := gitinfo.CurrentRevision(t.Context(), repo); after.Head != before.Head {
		t.Fatalf("stale destination was merged: %+v", after)
	}
	run(repo, "checkout", "-q", "integration")
	run(repo, "commit", "--allow-empty", "-qm", "same destination advances")
	request(http.MethodPost, "/merge-branches", body, http.StatusOK)
	if content, err := os.ReadFile(filepath.Join(parent, "file.txt")); err != nil || string(content) != "two\n" {
		t.Fatalf("parent dirty work changed: %q, %v", content, err)
	}
	if info := gitinfo.Inspect(t.Context(), parent); info.Dirty == nil || !*info.Dirty || info.Head != before.Head {
		t.Fatalf("alternate merge committed the parent's work: %+v", info)
	}
	if content, err := os.ReadFile(filepath.Join(repo, "worker.txt")); err != nil || string(content) != "committed\n" {
		t.Fatalf("merged worker.txt = %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "draft.txt")); !os.IsNotExist(err) {
		t.Fatalf("alternate source's uncommitted file reached destination: %v", err)
	}
	if info := gitinfo.Inspect(t.Context(), child); info.Dirty == nil || !*info.Dirty {
		t.Fatalf("alternate worktree dirty changes were committed: %+v", info)
	}
	if got, err := store.LoadSession(session.ID); err != nil || got.RuntimeRef.Cwd != parent {
		t.Fatalf("parent working directory changed: %+v, %v", got, err)
	}
	request(http.MethodPost, "/merge-branches", body, http.StatusBadRequest)
	loadBranches()
	if len(view.Branches) != 0 {
		t.Fatalf("merged branch still offered: %+v", view)
	}
}

func TestLocalCheckoutMergeSelectedWorktreeBranch(t *testing.T) {
	workspace, repo, child, store, _ := managedWorktreeSession(t, "worker", "acp")
	if err := gitinfo.CommitAll(t.Context(), child, "worker changes"); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:       "local-checkout",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: repo},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, Workspace: workspace}).Handler()
	request := func(method, action string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(method, "/v1/sessions/"+session.ID+"/repo"+action, strings.NewReader(string(body))))
		if res.Code != http.StatusOK {
			t.Fatalf("%s %s = %d: %s", method, action, res.Code, res.Body.String())
		}
		return res
	}
	var info gitinfo.Info
	if err := json.Unmarshal(request(http.MethodGet, "", nil).Body.Bytes(), &info); err != nil || info.MainBranch != "main" || info.IsWorktree {
		t.Fatalf("local checkout status = %+v, %v", info, err)
	}
	var view gitinfo.MergeBranchesView
	if err := json.Unmarshal(request(http.MethodGet, "/merge-branches", nil).Body.Bytes(), &view); err != nil || len(view.Branches) != 1 {
		t.Fatalf("local checkout sources = %+v, %v", view, err)
	}
	body, err := json.Marshal(map[string]any{"source": view.Branches[0].Revision, "target": view.Target})
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/merge-branches", body)
	if content, err := os.ReadFile(filepath.Join(repo, "file.txt")); err != nil || string(content) != "two\n" {
		t.Fatalf("local checkout did not receive selected changes: %q, %v", content, err)
	}
}
