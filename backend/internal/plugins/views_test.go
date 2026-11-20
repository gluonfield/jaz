package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
)

func TestOpeningViewSavesOnlyChangedLatestHTML(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	packages := New(layout)
	if _, err := packages.Install(archive(t, map[string]string{
		"example/plugin.json":       portable(t, "example", nil),
		"example/views/status.html": "<p>Status</p>",
		"example/views/quiz.html":   "<p>Quiz A</p>",
	})); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	thread, err := store.CreateSession(storage.CreateSession{Slug: "sibling-view"})
	if err != nil {
		t.Fatal(err)
	}
	presenter := &visualize.Service{Store: store, Sessions: store, Events: sessionevents.New()}
	caller := sessionevents.PluginView{Reference: "example/status", File: "status.json", Title: "Status", WidgetCode: "<p>Status</p>"}
	if _, err := presenter.Present(thread.ID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &caller}); err != nil {
		t.Fatal(err)
	}
	service := NewViewService(store, presenter, packages, layout)
	request := ViewRequest{Reference: caller.Reference, File: caller.File, Name: "quiz", Title: "Quiz"}
	open := func(html string, count int) sessionevents.PluginView {
		t.Helper()
		got, err := service.Open(t.Context(), thread.ID, request)
		if err != nil || got.WidgetCode != html || got.File != caller.File || got.Reference != "example/quiz" {
			t.Fatalf("opened=%+v, %v", got, err)
		}
		view, err := store.LoadSessionOverview(t.Context(), thread.ID)
		if err != nil || len(view.PresentationEvents) != count+1 {
			t.Fatalf("saved sibling events=%d, want %d; %v", len(view.PresentationEvents)-1, count, err)
		}
		return got
	}
	first := open("<p>Quiz A</p>", 1)
	request.Title = "Another label"
	if got := open("<p>Quiz A</p>", 1); got != first {
		t.Fatalf("unchanged view did not return its saved presentation: %+v", got)
	}
	path := filepath.Join(layout.Plugins, "example/views/quiz.html")
	if err := os.WriteFile(path, []byte("<p>Quiz B</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	open("<p>Quiz B</p>", 2)
	open("<p>Quiz B</p>", 2)
	if err := os.WriteFile(path, []byte("<p>Quiz A</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	open("<p>Quiz A</p>", 3)
}
