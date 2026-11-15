package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/skills"
)

func portable(t *testing.T, name string, jaz map[string]any) string {
	t.Helper()
	fields := map[string]any{"$schema": manifest.Schema, "name": name}
	if jaz != nil {
		fields["extensions"] = map[string]any{"chat.jaz": jaz}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// A repository's downloaded zip nests its plugins under a top folder.
var repository = map[string]string{
	"sg-main/README.md":                                   "# plugins",
	"sg-main/plugins/tenders/plugin.json":                 `{"name":"tenders","description":"Query sheets","version":"0.2.0","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tenders"}}}`,
	"sg-main/plugins/tenders/skills/query-sheet/SKILL.md": "---\nname: query-sheet\ndescription: Draft the query sheet\n---\n",
	"sg-main/plugins/tenders/views/status.html":           "<p id=status></p>",
	"sg-main/plugins/tenders/views/question.html":         "<p id=question></p>",
	"sg-main/plugins/tenders/scripts/status.py":           "print('ok')",
	"sg-main/plugins/quality/plugin.json":                 `{"name":"quality","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Quality"}}}`,
	"sg-main/plugins/quality/skills/inspection/SKILL.md":  "---\nname: inspection\ndescription: Write an inspection report\n---\n",
}

func TestInstallAddsEveryPluginInARepositoryArchive(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := store.Install(archive(t, repository)); err != nil {
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
	if tenders.DisplayName != "Tenders" || tenders.Version != "0.2.0" || !slices.Equal(tenders.Skills, []string{"tenders:query-sheet"}) || !slices.Equal(tenders.Views, []string{"question", "status"}) {
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
	store := New(runtimefiles.New(t.TempDir()))
	if _, err := store.Install(archive(t, repository)); err != nil {
		t.Fatal(err)
	}
	newer := archive(t, map[string]string{"tenders/plugin.json": `{"name":"tenders","version":"0.3.0","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tenders"}}}`})
	if _, err := store.Install(newer); err != nil {
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

func TestInstallRefusesWhatIsNotAPlugin(t *testing.T) {
	store := New(runtimefiles.New(t.TempDir()))
	for label, files := range map[string]map[string]string{
		"no manifest":   {"tenders/skills/query-sheet/SKILL.md": "x"},
		"unsafe name":   {"x/plugin.json": `{"name":"../skills","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"}`},
		"leaving paths": {"../plugin.json": `{"name":"tenders","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"}`},
	} {
		if _, err := store.Install(archive(t, files)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want invalid", label, err)
		}
	}
	if _, err := store.View("../skills/x"); !errors.Is(err, ErrInvalid) {
		t.Errorf("view outside plugins = %v, want invalid", err)
	}
}

type presented struct {
	artifact *sessionevents.PresentationEvent
}

func (p *presented) Present(_ string, artifact *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error) {
	p.artifact = artifact
	return &mcp.CallToolResult{}, nil, nil
}

func TestShowViewPresentsTheViewWithItsData(t *testing.T) {
	store := New(runtimefiles.New(t.TempDir()))
	if _, err := store.Install(archive(t, repository)); err != nil {
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
	if shown.artifact.Kind != sessionevents.PresentationPluginView || shown.artifact.View.File != "status.json" || shown.artifact.Artifact != nil {
		t.Fatalf("panel view = %+v", shown.artifact)
	}
	for label, input := range map[string]ShowViewInput{
		"data and file": {View: "tenders/status", Title: "x", Data: map[string]any{}, File: "status.json"},
		"neither":       {View: "tenders/status", Title: "x"},
		"file outside":  {View: "tenders/status", Title: "x", File: "../status.json"},
		"unknown view":  {View: "tenders/missing", Title: "x", File: "status.json"},
		"missing title": {View: "tenders/status", File: "status.json"},
	} {
		if _, _, err := tools.ShowView(context.Background(), nil, input); err == nil {
			t.Errorf("%s: accepted", label)
		}
	}
}
