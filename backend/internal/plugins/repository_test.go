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
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testgit"
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

func TestRepositoryMarketplaceUpdateAndFailedReplacement(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		".agents/plugins/marketplace.json":        `{"plugins":[{"name":"alpha","source":"./plugins/alpha"},{"name":"beta","source":{"source":"local","path":"./plugins/beta"}},{"name":"script","source":{"source":"command","command":"never-run"}}],"owner":{"ignored":true},"policy":"ignored"}`,
		".claude-plugin/marketplace.json":         `{"plugins":[{"name":"excluded","source":"./plugins/excluded"}]}`,
		"plugins/alpha/plugin.json":               `{"name":"alpha"}`,
		"plugins/alpha/views/status.html":         "first",
		"plugins/beta/.claude-plugin/plugin.json": `{"name":"beta","version":"1"}`,
		"plugins/excluded/plugin.json":            `{"name":"excluded"}`,
	})
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)
	first := fixtureGit(t, repo, "rev-parse", "HEAD")
	ref := fixtureGit(t, repo, "branch", "--show-current")
	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(layout).WithStateStore(db)
	result, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: repo, Ref: ref})
	if err != nil || len(result.Plugins) != 2 || len(result.Skipped) != 1 || result.Skipped[0].Name != "script" {
		t.Fatalf("marketplace install = %+v %v", result, err)
	}
	for _, plugin := range result.Plugins {
		if plugin.Source == nil || plugin.Source.Repository != repo || plugin.Source.Ref != ref || plugin.Source.Commit != first {
			t.Fatalf("origin = %+v", plugin)
		}
	}
	fixtureFiles(t, repo, map[string]string{"plugins/alpha/views/status.html": "second"})
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)
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
	testgit.Snapshot(t, repo)
	candidate, err := readPackage(filepath.Join(repo, "plugins", "alpha"), store.dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	previous := *updated.Plugins[0].Source
	beta, err := readPackage(filepath.Join(repo, "plugins", "beta"), store.dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := stagePackages(repo, []preparedPlugin{
		{plugin: candidate, path: filepath.Join(repo, "plugins", "alpha"), source: &Source{Repository: repo, Ref: ref, Commit: fixtureGit(t, repo, "rev-parse", "HEAD")}, expected: previous.Commit},
		{plugin: beta, path: filepath.Join(repo, "plugins", "beta"), source: &Source{Repository: repo, Ref: ref, Commit: first}},
	}, filepath.Join(t.TempDir(), "ready"), store.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(prepared[1].path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.publish(t.Context(), prepared); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing staged package did not fail its real rename: %v", err)
	}
	view, viewErr := store.View("alpha/status")
	listed, listErr := store.List()
	if viewErr != nil || listErr != nil || view != "second" || *listed[0].Source != previous || listed[1].Source.Commit != first {
		t.Fatalf("failed rename changed last good package/origin: %+v %q %v %v", listed, view, listErr, viewErr)
	}
}

func TestRepositoryMaterializesInRepoSymlinksAndRejectsEscapes(t *testing.T) {
	// Simulates base44/skills layout where plugins/base44/skills/app -> ../../../skills/app
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		"skills/app/SKILL.md":                    "---\nname: base44-app\ndescription: Build Base44 apps\n---\n",
		"plugins/base44/.codex-plugin/plugin.json": `{"name":"base44","interface":{"displayName":"Base44"}}`,
	})
	skillsDir := filepath.Join(repo, "plugins", "base44", "skills")
	if err := os.MkdirAll(skillsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "skills", "app"), filepath.Join(skillsDir, "app")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)

	store := New(runtimefiles.New(t.TempDir()))
	result, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: repo})
	if err != nil || len(result.Plugins) != 1 || !slices.Equal(skillNames(result.Plugins[0].Skills), []string{"base44:base44-app"}) {
		t.Fatalf("symlink materialization failed = %+v, %v", result, err)
	}

	// Escaping symlink outside the repo must fail.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	badRepo := t.TempDir()
	fixtureGit(t, badRepo, "init", "--quiet")
	fixtureFiles(t, badRepo, map[string]string{"plugin.json": `{"name":"escape"}`})
	if err := os.Symlink(outside, filepath.Join(badRepo, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, badRepo, "add", "--all")
	testgit.Snapshot(t, badRepo)
	if _, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: badRepo}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("escaping symlink accepted: %v", err)
	}
}

func TestRepositorySourceConflictAndMarketplaceCatalogRefresh(t *testing.T) {
	repoA := makeGitRepo(t, map[string]string{
		".agents/plugins/marketplace.json": `{"plugins":[{"name":"alpha","source":"./plugins/alpha"}]}`,
		"plugins/alpha/plugin.json":        `{"name":"alpha","displayName":"Alpha A"}`,
		"plugins/alpha/.mcp.json":          `{"mcpServers":{"alpha":{"type":"http","url":"https://alpha.example/mcp"}}}`,
	})
	repoB := makeGitRepo(t, map[string]string{
		"plugins/alpha/plugin.json": `{"name":"alpha","displayName":"Alpha B"}`,
	})
	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(New(layout), db, nil, nil, nil, nil)

	src, err := svc.AddSource(t.Context(), RepositoryInput{Repository: repoA})
	if err != nil || !src.IsMarketplace || len(src.Plugins) != 1 || src.Plugins[0].ID != "alpha" {
		t.Fatalf("marketplace source = %+v, %v", src, err)
	}
	catalog, err := svc.List(t.Context(), false)
	if err != nil || len(catalog.Plugins) != 1 || catalog.Plugins[0].Installed || catalog.Plugins[0].Origin != "marketplace" {
		t.Fatalf("browse catalog = %+v, %v", catalog, err)
	}
	if catalog.Plugins[0].Auth == nil || catalog.Plugins[0].Auth.Kind != "mcp_oauth" || catalog.Plugins[0].Auth.Status != "available" {
		t.Fatalf("marketplace remote MCP auth = %+v", catalog.Plugins[0].Auth)
	}
	_, searchOut, err := svc.searchMCP(t.Context(), nil, SearchInput{Query: "alpha"})
	if err != nil || len(searchOut.Plugins) != 1 || searchOut.Plugins[0].ConnectLink != "jaz://plugin/alpha/connect" {
		t.Fatalf("marketplace search connect link = %+v, %v", searchOut, err)
	}
	if _, err := svc.Install(t.Context(), RepositoryInput{Repository: repoA, Paths: []string{"./plugins/alpha"}}); err != nil {
		t.Fatal(err)
	}
	// Installing same ID from a different repo must return ErrConflict.
	if _, err := svc.Install(t.Context(), RepositoryInput{Repository: repoB}); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-source conflict = %v, want ErrConflict", err)
	}
	// Push a new commit to repoA and refresh sources -> update_available becomes true.
	fixtureFiles(t, repoA, map[string]string{"plugins/alpha/plugin.json": `{"name":"alpha","displayName":"Alpha A2","version":"2.0.0"}`})
	fixtureGit(t, repoA, "add", "--all")
	testgit.Snapshot(t, repoA)
	if err := svc.RefreshSources(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(t.Context(), "alpha", false)
	if err != nil || got.Source == nil || !got.Source.UpdateAvailable {
		t.Fatalf("expected UpdateAvailable after remote commit: %+v, %v", got, err)
	}
}

func TestRepositoryFallbackDiscoveryAndSafeFailures(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, map[string]string{
		"bundle/one/.codex-plugin/plugin.json": `{"name":"one"}`,
		"bundle/two/plugin.json":               `{"name":"two"}`,
	})
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)
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
