package plugins

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "core.hooksPath=" + os.DevNull}, args...)...)
	command.Dir = dir
	data, err := command.Output()
	if err != nil {
		t.Fatalf("Git fixture operation failed: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func fixtureFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, body := range files {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepositoryMarketplaceUpdateAndMetadataRollback(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		".agents/plugins/marketplace.json":        `{"plugins":[{"name":"alpha","source":"./plugins/alpha"},{"name":"beta","source":{"source":"local","path":"./plugins/beta"}},{"name":"remote","source":{"source":"url","url":"https://example.invalid/plugin.git"}},{"name":"script","source":{"source":"command","command":"never-run"}}],"owner":{"ignored":true},"policy":"ignored"}`,
		".claude-plugin/marketplace.json":         `{"plugins":[{"name":"excluded","source":"./plugins/excluded"}]}`,
		"plugins/alpha/plugin.json":               portable(t, "alpha", nil),
		"plugins/alpha/views/status.html":         "first",
		"plugins/beta/.claude-plugin/plugin.json": `{"name":"beta","version":"1"}`,
		"plugins/excluded/plugin.json":            portable(t, "excluded", nil),
	})
	fixtureGit(t, repo, "add", "--all")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "Initial plugin fixture")
	first := fixtureGit(t, repo, "rev-parse", "HEAD")
	ref := fixtureGit(t, repo, "branch", "--show-current")
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	result, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: repo, Ref: ref})
	if err != nil || len(result.Plugins) != 2 || len(result.Skipped) != 2 || result.Skipped[0].Name != "remote" || result.Skipped[1].Name != "script" {
		t.Fatalf("marketplace install = %+v %v", result, err)
	}
	for _, plugin := range result.Plugins {
		if plugin.Source == nil || plugin.Source.Repository != repo || plugin.Source.Ref != ref || plugin.Source.Commit != first {
			t.Fatalf("origin = %+v", plugin)
		}
	}
	fixtureFiles(t, repo, map[string]string{"plugins/alpha/views/status.html": "second"})
	fixtureGit(t, repo, "add", "--all")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "Update plugin fixture")
	second := fixtureGit(t, repo, "rev-parse", "HEAD")
	updated, err := store.Update(t.Context(), "alpha")
	if err != nil || len(updated.Plugins) != 1 || updated.Plugins[0].Source.Commit != second {
		t.Fatalf("update = %+v %v", updated, err)
	}
	if view, err := store.View("alpha/status"); err != nil || view != "second" {
		t.Fatalf("updated view = %q %v", view, err)
	}
	listed, err := store.List()
	if err != nil || listed[1].Source.Commit != first {
		t.Fatalf("updating alpha also updated beta: %+v %v", listed, err)
	}
	fixtureFiles(t, repo, map[string]string{"plugins/alpha/views/status.html": "unpublished"})
	fixtureGit(t, repo, "add", "--all")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "Unpublished plugin fixture")
	observedSwap := false
	store.commitMetadata = func(staged, path string) error {
		data, err := os.ReadFile(filepath.Join(layout.Plugins, "alpha", "views", "status.html"))
		if err != nil {
			t.Fatal(err)
		}
		observedSwap = string(data) == "unpublished"
		return fs.ErrPermission
	}
	if _, err := store.Update(t.Context(), "alpha"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("metadata failure was ignored: %v", err)
	}
	store.commitMetadata = os.Rename
	if !observedSwap {
		t.Fatal("fault did not occur after the real package exchange")
	}
	listed, err = store.List()
	view, viewErr := store.View("alpha/status")
	if err != nil || viewErr != nil || view != "second" || listed[0].Source.Commit != second {
		t.Fatalf("failed publication changed last good package/origin: %+v %q %v %v", listed, view, err, viewErr)
	}
	if _, err := store.Install(archive(t, map[string]string{"alpha/plugin.json": portable(t, "alpha", nil), "alpha/views/status.html": "ZIP"})); err != nil {
		t.Fatal(err)
	}
	listed, err = store.List()
	if err != nil || listed[0].Source != nil || listed[1].Source == nil {
		t.Fatalf("ZIP did not clear only its replaced origin: %+v %v", listed, err)
	}
	if _, err := store.Update(t.Context(), "alpha"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ZIP plugin offered Git update: %v", err)
	}
}

func TestRepositoryFallbackDiscoveryAndSafeFailures(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		"bundle/one/.codex-plugin/plugin.json": `{"name":"one"}`,
		"bundle/two/plugin.json":               portable(t, "two", nil),
	})
	fixtureGit(t, repo, "add", "--all")
	fixtureGit(t, repo, "commit", "--quiet", "-m", "Plugin discovery fixture")
	store := New(runtimefiles.New(t.TempDir()))
	result, err := store.InstallRepository(context.Background(), RepositoryInput{Repository: repo})
	if err != nil || len(result.Plugins) != 2 {
		t.Fatalf("recursive discovery = %+v %v", result, err)
	}
	for _, input := range []RepositoryInput{{Repository: "--upload-pack=never"}, {Repository: "https://user:secret@example.invalid/plugins.git"}, {Repository: "ssh://user:secret@127.0.0.1:1/plugins.git"}, {Repository: repo, Ref: "--danger"}} {
		if _, err := store.InstallRepository(t.Context(), input); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe input = %v", err)
		}
	}
	if _, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: filepath.Join(repo, "missing")}); err == nil || !strings.Contains(err.Error(), "backend machine needs access") {
		t.Fatalf("inaccessible repository = %v", err)
	}
	listed, err := store.List()
	var names []string
	for _, plugin := range listed {
		names = append(names, plugin.Name)
	}
	if err != nil || !slices.Equal(names, []string{"one", "two"}) {
		t.Fatalf("failed fetch changed installed packages: %+v %v", listed, err)
	}
}
