package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/storage"
)

type pageSessions struct {
	threads []storage.Session
	err     error
}

func (s pageSessions) ListSessions(filter storage.SessionFilter) ([]storage.Session, error) {
	var matches []storage.Session
	for _, thread := range s.threads {
		if storage.SessionMatchesFilter(thread, filter) {
			matches = append(matches, thread)
		}
	}
	return matches, s.err
}

func TestPageGroupsFoldersAndReloadsTheirData(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	if _, err := store.Install(archive(t, map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tenders","page":{"view":"status","file":"status.json"}}}}`,
		"tenders/views/status.html": "<p>status</p>",
	})); err != nil {
		t.Fatal(err)
	}
	folders := make([]string, 4)
	for i := range folders {
		folders[i] = t.TempDir()
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(folders[0], "status.json"), `{"title":"Tender A"}`)
	write(filepath.Join(folders[1], "status.json"), `false`)
	write(filepath.Join(folders[2], "status.json"), `{broken`)
	thread := func(id, folder string, updated int64) storage.Session {
		return storage.Session{ID: id, Title: "Thread " + id, Status: storage.StatusIdle, UpdatedAt: time.Unix(updated, 0), RuntimeRef: &storage.RuntimeRef{Cwd: folder}}
	}
	child := thread("newest", folders[0]+string(filepath.Separator)+".", 6)
	child.ParentID = "old"
	child.SourceType = storage.SourceBotWorker
	archived := thread("archived", folders[1], 3)
	archived.Archived = true
	sessions := pageSessions{threads: []storage.Session{
		thread("old", folders[0], 1), thread("other", folders[1], 4),
		thread("invalid", folders[2], 8), thread("missing", folders[3], 9),
		thread("relative", "relative", 10), {ID: "no-runtime"}, child, archived,
	}}
	page, err := store.Page("tenders", sessions)
	if err != nil {
		t.Fatal(err)
	}
	if page.View != "<p>status</p>" || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	first := page.Items[0]
	if first.Folder != folders[0] || first.Data.(map[string]any)["title"] != "Tender A" || len(first.Threads) != 2 || first.Threads[0].ID != "newest" || first.Threads[1].ID != "old" {
		t.Fatalf("first folder = %+v", first)
	}
	if page.Items[1].Folder != folders[1] || page.Items[1].Data != false || len(page.Items[1].Threads) != 2 || page.Items[1].Threads[1].ID != "archived" || page.Items[1].Threads[0].Title != "Thread other" || page.Items[1].Threads[0].Status != storage.StatusIdle || !page.Items[1].Threads[0].UpdatedAt.Equal(time.Unix(4, 0)) {
		t.Fatalf("second folder = %+v", page.Items[1])
	}
	write(filepath.Join(folders[0], "status.json"), `{"title":"Updated"}`)
	write(filepath.Join(layout.Plugins, "tenders", "views", "status.html"), "<p>updated view</p>")
	page, err = store.Page("tenders", sessions)
	if err != nil || page.View != "<p>updated view</p>" || page.Items[0].Data.(map[string]any)["title"] != "Updated" {
		t.Fatalf("reloaded page = %+v, %v", page, err)
	}
	write(filepath.Join(layout.Plugins, "tenders", "plugin.json"), `{"name":"tenders","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tenders","page":{"view":"status","file":"next.json"}}}}`)
	page, err = store.Page("tenders", sessions)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("changed filename = %+v, %v", page, err)
	}
	encoded, err := json.Marshal(page)
	var wire map[string]json.RawMessage
	if err != nil || json.Unmarshal(encoded, &wire) != nil || string(wire["items"]) != "[]" {
		t.Fatalf("empty items = %s, %v", encoded, err)
	}
	if _, err := store.Page("tenders", pageSessions{err: errors.New("storage failed")}); err == nil {
		t.Fatal("page ignored session storage failure")
	}
}

func TestInstallRejectsInvalidPagePaths(t *testing.T) {
	for _, page := range []Page{
		{View: "status", File: "../status.json"},
		{View: "status", File: `nested\status.json`},
		{View: "status", File: "/status.json"},
		{View: "status", File: "nested/status.json"},
		{View: "status", File: `C:\status.json`},
		{View: "status", File: ".."},
		{View: "status", File: "status.json\x00"},
		{View: "status", File: " status.json"},
		{View: "../status", File: "status.json"},
		{View: "status.html", File: "status.json"},
		{View: "missing", File: "status.json"},
	} {
		t.Run(page.View+"/"+page.File, func(t *testing.T) {
			store := New(runtimefiles.New(t.TempDir()))
			manifest := portable(t, "tenders", map[string]any{"page": page})
			if _, err := store.Install(archive(t, map[string]string{"tenders/plugin.json": manifest, "tenders/views/status.html": "<p>status</p>"})); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted page %+v: %v", page, err)
			}
		})
	}
}

func TestInstallRejectsInvalidIconPaths(t *testing.T) {
	for _, icon := range []string{"assets/icon.svg", "./../icon.svg", "./assets/../icon.svg", "./assets/./icon.svg", "./assets//icon.svg", `./assets\icon.svg`, "/assets/icon.svg", "https://example.com/icon.svg", "./assets/icon.png", "./missing.svg"} {
		t.Run(icon, func(t *testing.T) {
			store := New(runtimefiles.New(t.TempDir()))
			if _, err := store.Install(archive(t, map[string]string{"tenders/plugin.json": portable(t, "tenders", map[string]any{"icon": icon})})); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted icon %q: %v", icon, err)
			}
		})
	}
}

func TestPageIconCannotFollowASymlinkOutsideThePlugin(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store := New(layout)
	installed, err := store.Install(archive(t, map[string]string{
		"tenders/plugin.json":       `{"name":"tenders","$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","extensions":{"chat.jaz":{"displayName":"Tenders","icon":"./icon.svg","page":{"view":"status","file":"status.json"}}}}`,
		"tenders/icon.svg":          `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"tenders/views/status.html": "<p>status</p>",
	}))
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
	if _, err := store.Page("tenders", pageSessions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("icon escaped plugin: %v", err)
	}
}
