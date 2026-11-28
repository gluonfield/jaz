package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

type pageSessions struct {
	views []storage.SessionPluginView
	err   error
}

func (s pageSessions) ListSessionPluginViews() ([]storage.SessionPluginView, error) {
	return s.views, s.err
}

type storedPresentation struct {
	store *sqlite.Store
}

func (p storedPresentation) PresentMCP(id string, presentation *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error) {
	err := p.store.AppendSessionEvents(id, sessionevents.Event{Type: sessionevents.TypeArtifact, Artifact: presentation})
	return &mcp.CallToolResult{}, nil, err
}

func TestPageFollowsStoredShowViewPresentations(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := installFiles(t, store, map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","page":{"view":"status"}}`,
		"tenders/views/status.html": "<p>status</p>",
		"tenders/views/other.html":  "<p>other</p>",
		"other/plugin.json":         `{"name":"other"}`,
		"other/views/status.html":   "<p>other plugin</p>",
	}); err != nil {
		t.Fatal(err)
	}
	sessions, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	tools := MCPTools{Plugins: store, Presenter: storedPresentation{sessions}}
	folder, other, incidental := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(folder, file, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(folder, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(folder, "status.json", `{"title":"Shared"}`)
	write(folder, "next.json", `{"title":"Latest"}`)
	write(folder, "invalid.json", `{broken`)
	write(other, "status.json", `false`)
	write(incidental, "status.json", `{"title":"Unrelated"}`)
	create := func(title, folder string) storage.Session {
		t.Helper()
		thread, err := sessions.CreateSession(storage.CreateSession{Title: title, RuntimeRef: &storage.RuntimeRef{Cwd: folder}})
		if err != nil {
			t.Fatal(err)
		}
		return thread
	}
	show := func(thread storage.Session, ref, file string) {
		t.Helper()
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(thread.ID)}}
		if _, _, err := tools.ShowView(t.Context(), req, ShowViewInput{View: ref, Title: thread.Title, File: file}); err != nil {
			t.Fatal(err)
		}
	}
	first := create("First", folder)
	show(first, "tenders/status", "status.json")
	show(first, "other/status", "next.json")
	time.Sleep(2 * time.Millisecond)
	shared, err := sessions.CreateSession(storage.CreateSession{Title: "Archived child", ParentID: first.ID, SourceType: "fixture", RuntimeRef: &storage.RuntimeRef{Cwd: folder + "/."}})
	if err != nil {
		t.Fatal(err)
	}
	show(shared, "tenders/other", "status.json")
	if err := sessions.SetArchived(shared.ID, true); err != nil {
		t.Fatal(err)
	}
	create("Incidental file", incidental)
	legacy := create("Old view without reference", incidental)
	if err := sessions.AppendSessionEvents(legacy.ID, sessionevents.Event{Type: sessionevents.TypeArtifact, Content: `{"title":"Old","widget_code":"<p>old</p>","file":"status.json"}`}); err != nil {
		t.Fatal(err)
	}
	events, err := sessions.LoadSessionEvents(legacy.ID)
	if err != nil || len(events) != 1 || events[0].Artifact == nil || events[0].Artifact.View == nil || events[0].Artifact.View.Reference != "" || events[0].Artifact.View.WidgetCode != "<p>old</p>" {
		t.Fatalf("old view no longer renders: %+v %v", events, err)
	}
	show(create("Invalid JSON", folder), "tenders/status", "invalid.json")
	show(create("Missing JSON", folder), "tenders/status", "missing.json")
	show(create("Relative folder", "relative"), "tenders/status", "status.json")
	outside := t.TempDir()
	write(outside, "escape.json", `{"title":"Outside"}`)
	if err := os.Symlink(filepath.Join(outside, "escape.json"), filepath.Join(folder, "escape.json")); err != nil {
		t.Fatal(err)
	}
	show(create("Escaping symlink", folder), "tenders/status", "escape.json")
	for _, file := range []string{"../status.json", `nested\status.json`, "/status.json", "nested/../status.json", "nested//status.json", `C:\status.json`, "..", "status.json\x00", " status.json"} {
		thread := create("Unsafe stored filename", folder)
		show(thread, "tenders/status", "status.json")
		if err := sessions.AppendSessionEvents(thread.ID, sessionevents.Event{Type: sessionevents.TypeArtifact, Artifact: &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &sessionevents.PluginView{Reference: "tenders/status", Title: "Unsafe", WidgetCode: "<p></p>", File: file}}}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(2 * time.Millisecond)
	show(create("Other folder", other), "tenders/status", "status.json")
	time.Sleep(2 * time.Millisecond)
	latest := create("Latest file", folder)
	show(latest, "tenders/status", "status.json")
	show(latest, "tenders/other", "next.json")
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header(latest.ID)}}
	if _, _, err := tools.ShowView(t.Context(), req, ShowViewInput{View: "tenders/status", Title: "Inline card", Data: map[string]any{"title": "Inline"}}); err != nil {
		t.Fatal(err)
	}
	page, err := store.Page("tenders", sessions)
	if err != nil || page.View != "<p>status</p>" || len(page.Items) != 3 {
		t.Fatalf("page = %+v %v", page, err)
	}
	if page.Items[0].Folder != folder || page.Items[0].Data.(map[string]any)["title"] != "Latest" || len(page.Items[0].Threads) != 1 || page.Items[0].Threads[0].ID != latest.ID || page.Items[1].Data != false || len(page.Items[2].Threads) != 2 || page.Items[2].Threads[0].ID != shared.ID {
		t.Fatalf("latest event, grouping, or ordering = %+v", page.Items)
	}
	for _, thread := range page.Items[2].Threads {
		if thread.ID != first.ID && thread.ID != shared.ID {
			t.Fatalf("unrelated thread linked: %+v", thread)
		}
	}
	write(folder, "next.json", `null`)
	write(layout.Plugins, "tenders/views/status.html", "<p>updated</p>")
	page, err = store.Page("tenders", sessions)
	if err != nil || page.View != "<p>updated</p>" || page.Items[0].Data != nil {
		t.Fatalf("file-only update not reflected: %+v %v", page, err)
	}
	write(folder, "missing.json", `"Created later"`)
	page, err = store.Page("tenders", sessions)
	if err != nil || len(page.Items) != 4 {
		t.Fatalf("newly created followed file was not read: %+v %v", page, err)
	}
	empty, err := store.Page("tenders", pageSessions{})
	encoded, marshalErr := json.Marshal(empty)
	if err != nil || marshalErr != nil || !strings.Contains(string(encoded), `"items":[]`) {
		t.Fatalf("empty page = %s %v %v", encoded, err, marshalErr)
	}
	if _, err := store.Page("tenders", pageSessions{err: errors.New("storage failed")}); err == nil {
		t.Fatal("page ignored session storage failure")
	}
}

func TestInstallRejectsInvalidPageViews(t *testing.T) {
	for _, view := range []string{"../status", "status.html", "missing"} {
		store := New(runtimefiles.New(t.TempDir()))
		definition := manifestJSON(t, map[string]any{"name": "tenders", "page": Page{View: view}})
		if _, err := installFiles(t, store, map[string]string{"tenders/plugin.json": definition, "tenders/views/status.html": "<p>status</p>"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted page view %q: %v", view, err)
		}
	}
}

func TestInstallAcceptsSVGAndPNGAndHTTPSIconsAndRejectsUnsafePaths(t *testing.T) {
	store := New(runtimefiles.New(t.TempDir()))
	installed, err := installFiles(t, store, map[string]string{
		"svg-plugin/plugin.json":  `{"name":"svg-plugin","logo":"./assets/icon.svg"}`,
		"svg-plugin/assets/icon.svg": `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"url-plugin/plugin.json":  `{"name":"url-plugin","logo":"https://example.com/icon.png"}`,
	})
	if err != nil || len(installed) != 2 {
		t.Fatalf("valid icons failed = %+v %v", installed, err)
	}
	for _, p := range installed {
		if p.Name == "svg-plugin" && !strings.HasPrefix(p.IconDataURL, "data:image/svg+xml;base64,") {
			t.Fatalf("svg icon = %+v", p.Icon)
		}
		if p.Name == "url-plugin" && p.Icon.Kind != "url" {
			t.Fatalf("url icon = %+v", p.Icon)
		}
	}
	for _, icon := range []string{"./../icon.svg", "./assets/../icon.svg", `./assets\icon.svg`, "/assets/icon.svg", "http://example.com/icon.svg", "./missing.svg"} {
		t.Run(icon, func(t *testing.T) {
			store := New(runtimefiles.New(t.TempDir()))
			if _, err := installFiles(t, store, map[string]string{"tenders/plugin.json": manifestJSON(t, map[string]any{"name": "tenders", "logo": icon})}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted icon %q: %v", icon, err)
			}
		})
	}
}

func TestIconCannotFollowASymlinkOutsideThePlugin(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	installed, err := installFiles(t, store, map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","displayName":"Tenders","logo":"./icon.svg","page":{"view":"status"}}`,
		"tenders/icon.svg":          `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"tenders/views/status.html": "<p>status</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := manifest.Read(filepath.Join(layout.Plugins, "tenders"))
	if err != nil || definition.IconPath != "./icon.svg" || !strings.HasPrefix(installed[0].IconDataURL, "data:image/svg+xml;base64,") {
		t.Fatalf("icon path and display data were conflated: %+v", installed[0])
	}
	icon := filepath.Join(layout.Plugins, "tenders", "icon.svg")
	outside := filepath.Join(layout.Root, "outside.svg")
	if err := os.Rename(icon, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, icon); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.List(); err != nil || len(listed) != 0 {
		t.Fatalf("icon escaped plugin: %+v %v", listed, err)
	}
	if page, err := store.Page("tenders", pageSessions{}); err != nil || page.View != "<p>status</p>" {
		t.Fatalf("page depended on unrelated icon loading: %+v %v", page, err)
	}
}
