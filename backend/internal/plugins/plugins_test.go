package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/skills"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/testgit"
)

func manifestJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func makeGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--quiet")
	fixtureFiles(t, repo, files)
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)
	return repo
}

func installFiles(t *testing.T, store *Store, files map[string]string) ([]Plugin, error) {
	t.Helper()
	repo := makeGitRepo(t, files)
	result, err := store.InstallRepository(context.Background(), RepositoryInput{Repository: repo})
	return result.Plugins, err
}

func skillNames(list []PluginSkill) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Name
	}
	return out
}

var repositoryFixture = map[string]string{
	"README.md":                                   "# plugins",
	"plugins/tenders/plugin.json":                 `{"name":"tenders","displayName":"Tenders","description":"Query sheets","version":"0.2.0"}`,
	"plugins/tenders/skills/query-sheet/SKILL.md": "---\nname: query-sheet\ndescription: Draft the query sheet\n---\n",
	"plugins/tenders/views/status.html":           "<p id=status></p>",
	"plugins/tenders/views/question.html":         "<p id=question></p>",
	"plugins/tenders/scripts/status.py":           "print('ok')",
	"plugins/quality/plugin.json":                 `{"name":"quality","displayName":"Quality"}`,
	"plugins/quality/skills/inspection/SKILL.md":  "---\nname: inspection\ndescription: Write an inspection report\n---\n",
}

func TestInstallAddsEveryPluginInARepository(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := installFiles(t, store, repositoryFixture); err != nil {
		t.Fatal(err)
	}
	installed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 || installed[1].Name != "tenders" {
		t.Fatalf("installed = %+v, want quality and tenders", installed)
	}
	tenders := installed[1]
	if tenders.DisplayName != "Tenders" || tenders.Version != "0.2.0" || !slices.Equal(skillNames(tenders.Skills), []string{"tenders:query-sheet"}) || !slices.Equal(tenders.Views, []string{"question", "status"}) {
		t.Fatalf("tenders = %+v", tenders)
	}
	html, err := store.View("tenders/status")
	if err != nil || html != "<p id=status></p>" {
		t.Fatalf("view = %q, %v", html, err)
	}
	catalog, err := skills.Load(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(catalog.Skills))
	for i, skill := range catalog.Skills {
		names[i] = skill.Name
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"quality:inspection", "tenders:query-sheet"}) {
		t.Fatalf("agent skills = %v, want the plugins' skills", names)
	}
}

func TestInstallReplacesAndRemoveDeletes(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	db, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(layout).WithStateStore(db)
	repo := makeGitRepo(t, repositoryFixture)
	if _, err := store.InstallRepository(t.Context(), RepositoryInput{Repository: repo}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "plugins", "tenders", "views", "status.html")); err != nil {
		t.Fatal(err)
	}
	fixtureFiles(t, repo, map[string]string{
		"plugins/tenders/plugin.json": `{"name":"tenders","displayName":"Tenders","version":"0.3.0"}`,
	})
	fixtureGit(t, repo, "add", "--all")
	testgit.Snapshot(t, repo)
	if _, err := store.Update(t.Context(), "tenders"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.View("tenders/status"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old view after replacing = %v, want not found", err)
	}
	if err := store.Remove("tenders"); err != nil {
		t.Fatal(err)
	}
	installed, _ := store.List()
	if len(installed) != 1 || installed[0].Name != "quality" {
		t.Fatalf("after remove = %+v, want only quality", installed)
	}
	if err := store.Remove("tenders"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing twice = %v, want not found", err)
	}
}

func TestInstallRefusesInvalidNamesAndReservedIDs(t *testing.T) {
	store := New(runtimefiles.New(t.TempDir())).WithReservedIDs(func(id string) bool { return id == "gmail" })
	for label, files := range map[string]map[string]string{
		"no plugin or skills": {"README.md": "hello"},
		"unsafe name":         {"plugin.json": `{"name":"../skills"}`},
		"reserved builtin id": {"plugin.json": `{"name":"gmail"}`},
	} {
		if _, err := installFiles(t, store, files); err == nil {
			t.Errorf("%s: unexpectedly accepted", label)
		}
	}
	if _, err := store.View("../skills/x"); !errors.Is(err, ErrInvalid) {
		t.Errorf("view outside plugins = %v, want invalid", err)
	}
}

type presented struct {
	artifact *sessionevents.PresentationEvent
}

func (p *presented) PresentMCP(_ string, artifact *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error) {
	p.artifact = artifact
	return &mcp.CallToolResult{}, nil, nil
}

func TestShowViewPresentsTheViewWithItsData(t *testing.T) {
	store := New(runtimefiles.New(t.TempDir()))
	if _, err := installFiles(t, store, repositoryFixture); err != nil {
		t.Fatal(err)
	}
	shown := &presented{}
	tools := &MCPTools{Plugins: store, Presenter: shown}
	if _, _, err := tools.ShowView(context.Background(), nil, ShowViewInput{View: "tenders/question", Title: "Point 9", Data: map[string]any{"number": 9}}); err != nil {
		t.Fatal(err)
	}
	if shown.artifact.Kind != sessionevents.PresentationArtifact || shown.artifact.Artifact.WidgetCode != "<p id=question></p>" || string(shown.artifact.Artifact.Data) != `{"number":9}` || shown.artifact.View != nil {
		t.Fatalf("card = %+v", shown.artifact)
	}
	if _, _, err := tools.ShowView(context.Background(), nil, ShowViewInput{View: "tenders/status", Title: "Tender status", File: "status.json"}); err != nil {
		t.Fatal(err)
	}
	if shown.artifact.Kind != sessionevents.PresentationPluginView || shown.artifact.View.File != "status.json" || shown.artifact.View.Reference != "tenders/status" || shown.artifact.Artifact != nil {
		t.Fatalf("panel view = %+v", shown.artifact)
	}
	if _, _, err := tools.ShowView(context.Background(), nil, ShowViewInput{View: "tenders/status", Title: "Tender status", File: "runs/scene.json"}); err != nil {
		t.Fatal(err)
	}
	if shown.artifact.Kind != sessionevents.PresentationPluginView || shown.artifact.View.File != "runs/scene.json" || shown.artifact.View.Reference != "tenders/status" || shown.artifact.Artifact != nil {
		t.Fatalf("nested panel view = %+v", shown.artifact)
	}
	for label, input := range map[string]ShowViewInput{
		"data and file":    {View: "tenders/status", Title: "x", Data: map[string]any{}, File: "status.json"},
		"neither":          {View: "tenders/status", Title: "x"},
		"file traverse":    {View: "tenders/status", Title: "x", File: "nested/../status.json"},
		"file double slsh": {View: "tenders/status", Title: "x", File: "nested//status.json"},
		"file absolute":    {View: "tenders/status", Title: "x", File: "/nested/status.json"},
		"file dot segment": {View: "tenders/status", Title: "x", File: "./status.json"},
		"file hidden dir":  {View: "tenders/status", Title: "x", File: ".git/config"},
		"file windows":     {View: "tenders/status", Title: "x", File: `nested\status.json`},
		"file outside":     {View: "tenders/status", Title: "x", File: "../status.json"},
		"unknown view":     {View: "tenders/missing", Title: "x", File: "status.json"},
		"missing title":    {View: "tenders/status", File: "status.json"},
	} {
		if _, _, err := tools.ShowView(context.Background(), nil, input); err == nil {
			t.Errorf("%s: accepted", label)
		}
	}
}
