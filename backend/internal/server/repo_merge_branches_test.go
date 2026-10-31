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
)

func TestSessionMergeSelectedWorktreeBranch(t *testing.T) {
	workspace, repo, parent, store, session := managedWorktreeSession(t, "parent", "acp")
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(parent, "restore", "file.txt")
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
	var branches []gitinfo.MergeBranch
	if err := json.Unmarshal(request(http.MethodGet, "/merge-branches", "", http.StatusOK).Body.Bytes(), &branches); err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 || branches[0].Branch != "jaz/worker" || branches[0].Ahead != 1 {
		t.Fatalf("branches = %+v, want worker commit against integration", branches)
	}
	for _, body := range []string{`{"branch":"unlinked"}`, `{"branch":"integration"}`, `{"branch":"--all"}`, `{"branch":""}`, `{"branch":42}`} {
		request(http.MethodPost, "/merge", body, http.StatusBadRequest)
	}
	request(http.MethodPost, "/merge", `{"branch":"jaz/worker"}`, http.StatusOK)
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
	request(http.MethodPost, "/merge", `{"branch":"jaz/worker"}`, http.StatusBadRequest)
	if err := json.Unmarshal(request(http.MethodGet, "/merge-branches", "", http.StatusOK).Body.Bytes(), &branches); err != nil || len(branches) != 0 {
		t.Fatalf("merged branch still offered: %+v, %v", branches, err)
	}
}
