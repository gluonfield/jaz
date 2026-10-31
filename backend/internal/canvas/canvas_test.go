package canvas

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(runtimefiles.New(t.TempDir()))
}

func writeCanvas(t *testing.T, service *Service, session, content string) {
	t.Helper()
	dir := service.Dir(session)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPatchMergesFieldsIntoTheAgentsBoard(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"nodes":[
		{"id":"a","type":"text","text":"Agent note","x":0,"y":0,"width":200,"height":100,"styleAttributes":{"shape":"pill"}},
		{"id":"b","type":"text","text":"Other","x":300,"y":0,"width":200,"height":100}
	],"edges":[{"id":"e","fromNode":"a","toNode":"b","label":"next"}]}`)

	if _, err := service.Patch("s1", Patch{
		Page:  "page-1",
		Nodes: []Item{{"id": "a", "x": 40.0, "y": 60.0}, {"id": "c", "type": "group", "label": "Section", "x": -40.0, "y": -40.0, "width": 600.0, "height": 240.0}},
		Edges: []Item{{"id": "e", "label": nil}},
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	view := snapshot.View().Pages[0]
	a := view.Nodes[0]
	if a["x"] != 40.0 || a["y"] != 60.0 || a["text"] != "Agent note" || a["styleAttributes"] == nil {
		t.Fatalf("patched node lost agent fields or kept old position: %#v", a)
	}
	if len(view.Nodes) != 3 || view.Nodes[2]["id"] != "c" {
		t.Fatalf("new node should be appended: %#v", view.Nodes)
	}
	if _, ok := view.Edges[0]["label"]; ok {
		t.Fatalf("null field should be removed: %#v", view.Edges[0])
	}

	raw, err := os.ReadFile(filepath.Join(service.Dir("s1"), FileName))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	whole := false
	for _, line := range lines {
		if strings.Contains(line, `"id":"a"`) && strings.Contains(line, `"text":"Agent note"`) && json.Valid([]byte(strings.TrimSuffix(strings.TrimSpace(line), ","))) {
			whole = true
		}
	}
	if !whole {
		t.Fatalf("each item should sit whole on its own line:\n%s", raw)
	}
}

func TestRemovingANodeDropsItsEdges(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"nodes":[{"id":"a","type":"text"},{"id":"b","type":"text"},{"id":"c","type":"text"}],
		"edges":[{"id":"ab","fromNode":"a","toNode":"b"},{"id":"bc","fromNode":"b","toNode":"c"}]}`)

	if _, err := service.Patch("s1", Patch{Page: "page-1", Remove: []string{"a"}}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	view := snapshot.View().Pages[0]
	if len(view.Nodes) != 2 || len(view.Edges) != 1 || view.Edges[0]["id"] != "bc" {
		t.Fatalf("nodes %#v edges %#v", view.Nodes, view.Edges)
	}
}

func TestInvalidBoardIsReportedNotOverwritten(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"nodes":[{"id":"a"`)

	if _, err := service.Load("s1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Load error = %v, want ErrInvalid", err)
	}
	if _, err := service.Patch("s1", Patch{Page: "page-1", Nodes: []Item{{"id": "b"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Patch error = %v, want ErrInvalid", err)
	}
	raw, _ := os.ReadFile(filepath.Join(service.Dir("s1"), FileName))
	if string(raw) != `{"nodes":[{"id":"a"` {
		t.Fatalf("a broken board must stay as the agent wrote it, got %q", raw)
	}
}

func TestComponentsCompileAndReportErrors(t *testing.T) {
	service := newTestService(t)
	dir := service.Dir("s1")
	if err := os.MkdirAll(filepath.Join(dir, "components"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "components", "ok.tsx"), []byte(`export default function Ok() { const n: number = 2; return <b>{n}</b> }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "components", "broken.tsx"), []byte(`export default function Broken() { return <b> }`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCanvas(t, service, "s1", `{"nodes":[
		{"id":"ok","type":"file","file":"components/ok.tsx"},
		{"id":"broken","type":"file","file":"components/broken.tsx"},
		{"id":"gone","type":"file","file":"/nowhere/missing.png"}
	]}`)

	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	view := snapshot.View().Pages[0]
	ok := view.Components["ok"]
	if ok.Error != "" || !strings.Contains(ok.Code, `require("react/jsx-runtime")`) || strings.Contains(ok.Code, ": number") {
		t.Fatalf("ok component = %#v", ok)
	}
	if broken := view.Components["broken"]; broken.Code != "" || !strings.Contains(broken.Error, "broken.tsx:1:") {
		t.Fatalf("broken component = %#v", broken)
	}
	if file := view.Files["gone"]; file.Version != "" || file.Path != "/nowhere/missing.png" {
		t.Fatalf("missing file = %#v", file)
	}
	if _, compiled := view.Components["gone"]; compiled {
		t.Fatal("missing files are not compiled")
	}
}

func TestVersionChangesWhenAReferencedFileChanges(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"nodes":[{"id":"doc","type":"file","file":"notes.md"}]}`)
	path := filepath.Join(service.Dir("s1"), "notes.md")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("two, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if before.Version == after.Version {
		t.Fatal("editing a file on the board must change the board version")
	}
}

func TestLegacyBoardLoadsWithoutWritingAndMigratesOnPatch(t *testing.T) {
	service := newTestService(t)
	legacy := `{"nodes":[{"id":" ","type":"text","text":"Existing","custom":{"kept":true}}],"edges":[]}`
	writeCanvas(t, service, "s1", legacy)
	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	page := snapshot.View().Pages[0]
	if page.ID != "page-1" || page.Name != "Page 1" || page.Nodes[0]["text"] != "Existing" {
		t.Fatalf("legacy page = %#v", page)
	}
	path := filepath.Join(service.Dir("s1"), FileName)
	if raw, _ := os.ReadFile(path); string(raw) != legacy {
		t.Fatalf("loading rewrote legacy board: %s", raw)
	}
	if _, err := service.Patch("s1", Patch{Page: page.ID, Nodes: []Item{{"id": " ", "x": 42.0}}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document Document
	if err := json.Unmarshal(raw, &document); err != nil || len(document.Pages) != 1 {
		t.Fatalf("migrated document = %s, err = %v", raw, err)
	}
	if node := document.Pages[0].Nodes[0]; node["text"] != "Existing" || node["custom"] == nil || node["x"] != 42.0 {
		t.Fatalf("migration lost content: %#v", node)
	}
}

func TestPagesPersistNamesAndIsolateItemEdits(t *testing.T) {
	service := newTestService(t)
	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if pages := snapshot.View().Pages; len(pages) != 1 || pages[0].ID != "page-1" || pages[0].Name != "Page 1" {
		t.Fatalf("initial pages = %#v", pages)
	}
	if _, err := os.Stat(filepath.Join(service.Dir("s1"), FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initial read created a file: %v", err)
	}
	for _, change := range []Patch{
		{Page: "page-1", Nodes: []Item{{"id": "a", "text": "Original"}, {"id": "b"}}, Edges: []Item{{"id": "edge", "fromNode": "a", "toNode": "b"}}},
		{Page: "second page", Create: true, Nodes: []Item{{"id": "a", "text": "Second"}, {"id": "b"}}, Edges: []Item{{"id": "edge", "fromNode": "a", "toNode": "b"}}},
	} {
		if _, err := service.Patch("s1", change); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err = service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if page := snapshot.View().Pages[1]; page.ID != "second page" || page.Name != "Page 2" {
		t.Fatalf("new page = %#v", page)
	}
	name := "  Page 3  "
	if _, err := service.Patch("s1", Patch{Page: "second page", Name: &name, Remove: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Patch("s1", Patch{Page: "third", Create: true}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	pages := snapshot.View().Pages
	if len(pages) != 3 || pages[1].ID != "second page" || pages[1].Name != "Page 3" || pages[2].Name != "Page 4" {
		t.Fatalf("page names = %#v", pages)
	}
	if len(pages[0].Nodes) != 2 || len(pages[0].Edges) != 1 || pages[0].Nodes[0]["text"] != "Original" {
		t.Fatalf("second-page removal changed the first: %#v", pages[0])
	}
	if len(pages[1].Nodes) != 1 || len(pages[1].Edges) != 0 {
		t.Fatalf("page-scoped removal missed its edge: %#v", pages[1])
	}
}

func TestInvalidPageWritesLeaveDocumentUntouched(t *testing.T) {
	service := newTestService(t)
	if _, err := service.Patch("s1", Patch{Page: "page-1", Nodes: []Item{{"id": "kept", "text": "Kept"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(service.Dir("s1"), FileName)
	before, _ := os.ReadFile(path)
	blank := "   "
	for _, patch := range []Patch{
		{Nodes: []Item{{"id": "wrong"}}},
		{Page: "   "},
		{Page: " padded", Create: true},
		{Page: "padded ", Create: true},
		{Page: "\ufeffpadded", Create: true},
		{Page: "page-1 ", Nodes: []Item{{"id": "wrong"}}},
		{Page: "unknown", Nodes: []Item{{"id": "wrong"}}},
		{Page: "page-1", Create: true},
		{Page: "page-1", Delete: true},
		{Page: "unknown", Delete: true},
		{Page: "page-1", Name: &blank},
		{Page: "new", Create: true, Nodes: []Item{{"id": "a"}}, Edges: []Item{{"id": "a"}}},
	} {
		if _, err := service.Patch("s1", patch); !errors.Is(err, ErrInvalid) {
			t.Fatalf("patch %#v error = %v", patch, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Fatalf("invalid patch changed document: %s", after)
		}
	}
}

func TestPageDeletionPreservesOtherPages(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"pages":[
		{"id":"kept","name":"Kept","nodes":[{"id":"same","text":"Keep","custom":42}],"edges":[]},
		{"id":"deleted","name":"Deleted","nodes":[{"id":"same","text":"Remove"}],"edges":[]}
	]}`)
	path := filepath.Join(service.Dir("s1"), FileName)
	before, _ := os.ReadFile(path)
	name := "Name"
	for _, change := range []Patch{
		{Page: "deleted", Delete: true, Create: true},
		{Page: "deleted", Delete: true, Name: &name},
		{Page: "deleted", Delete: true, Nodes: []Item{{"id": "new"}}},
		{Page: "deleted", Delete: true, Edges: []Item{{"id": "new"}}},
		{Page: "deleted", Delete: true, Remove: []string{"same"}},
	} {
		if _, err := service.Patch("s1", change); !errors.Is(err, ErrInvalid) {
			t.Fatalf("mixed deletion %#v error = %v", change, err)
		}
		if after, _ := os.ReadFile(path); string(after) != string(before) {
			t.Fatalf("rejected deletion changed the document: %s", after)
		}
	}
	version, err := service.Patch("s1", Patch{Page: "deleted", Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	pages := snapshot.View().Pages
	if snapshot.Version != version || len(pages) != 1 || pages[0].ID != "kept" || pages[0].Name != "Kept" || pages[0].Nodes[0]["text"] != "Keep" || pages[0].Nodes[0]["custom"] != 42.0 {
		t.Fatalf("deletion lost a remaining page or returned a stale version: %#v", snapshot.View())
	}
}

func TestInvalidPageDocumentsAreNotOverwritten(t *testing.T) {
	service := newTestService(t)
	for _, raw := range []string{
		`{"pages":[]}`,
		`{"pages":null}`,
		`{"pages":[{"id":""}]}`,
		`{"pages":[{"id":" p"}]}`,
		`{"pages":[{"id":"p\u00a0"}]}`,
		`{"pages":[{"id":"\ufeffp"}]}`,
		`{"pages":[{"id":"same"},{"id":"same"}]}`,
		`{"pages":[{"id":"p","nodes":[{"id":"a"},{"id":"a"}]}]}`,
		`{"pages":[{"id":"p"}],"nodes":[{"id":"lost"}]}`,
	} {
		writeCanvas(t, service, "s1", raw)
		if _, err := service.Load("s1"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("load %s error = %v", raw, err)
		}
		if _, err := service.Patch("s1", Patch{Page: "p", Nodes: []Item{{"id": "changed"}}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("patch %s error = %v", raw, err)
		}
		if after, _ := os.ReadFile(filepath.Join(service.Dir("s1"), FileName)); string(after) != raw {
			t.Fatalf("invalid document overwritten: %s", after)
		}
	}
}

func TestReferencedFilesAndComponentsAreScopedToTheirPage(t *testing.T) {
	service := newTestService(t)
	writeCanvas(t, service, "s1", `{"pages":[
		{"id":"one","nodes":[{"id":"shared","type":"file","file":"one.tsx"}]},
		{"id":"two","name":"Named","nodes":[{"id":"shared","type":"file","file":"two.tsx"}]}
	]}`)
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(service.Dir("s1"), name+".tsx"), []byte(`export default function Card() { return <b>`+name+`</b> }`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	pages := snapshot.View().Pages
	if pages[0].Name != "Page 1" || pages[1].Name != "Named" {
		t.Fatalf("normalized page names = %#v", pages)
	}
	for i, name := range []string{"one", "two"} {
		if page := pages[i]; page.Files["shared"].Path != filepath.Join(service.Dir("s1"), name+".tsx") || !strings.Contains(page.Components["shared"].Code, name) {
			t.Fatalf("page %s has another page's file or component: %#v", name, page)
		}
	}
	if err := os.WriteFile(filepath.Join(service.Dir("s1"), "two.tsx"), []byte(`export default () => <b>Changed second page</b>`), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := service.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version == after.Version || pages[0].Files["shared"].Version != after.View().Pages[0].Files["shared"].Version {
		t.Fatal("a second-page file change must change the document version without changing the first-page file")
	}
}
