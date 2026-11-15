package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestListSessionPluginViewsReturnsOnlyPluginViewsOfThreadsWithFolders(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	folder := t.TempDir()
	thread, err := store.CreateSession(storage.CreateSession{Title: "Tender", RuntimeRef: &storage.RuntimeRef{Cwd: folder}})
	if err != nil {
		t.Fatal(err)
	}
	homeless, err := store.CreateSession(storage.CreateSession{Title: "No folder"})
	if err != nil {
		t.Fatal(err)
	}
	view := &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &sessionevents.PluginView{Reference: "tenders/status", Title: "Status", WidgetCode: "<p>status</p>", File: "status.json"}}
	widget := &sessionevents.PresentationEvent{Kind: sessionevents.PresentationArtifact, Artifact: &sessionevents.ArtifactEvent{Title: "Chart", WidgetCode: "<p>chart</p>"}}
	canvas := &sessionevents.PresentationEvent{Kind: sessionevents.PresentationCanvas, Presented: true}
	for _, artifact := range []*sessionevents.PresentationEvent{widget, view, canvas} {
		if err := store.AppendSessionEvents(thread.ID, sessionevents.Event{SessionID: thread.ID, Type: sessionevents.TypeArtifact, Artifact: artifact}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AppendSessionEvents(homeless.ID, sessionevents.Event{SessionID: homeless.ID, Type: sessionevents.TypeArtifact, Artifact: view}); err != nil {
		t.Fatal(err)
	}
	views, err := store.ListSessionPluginViews()
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %+v, %v", views, err)
	}
	got := views[0]
	if got.ID != thread.ID || got.Title != "Tender" || got.Folder != folder || got.Reference != "tenders/status" || got.File != "status.json" {
		t.Fatalf("view = %+v", got)
	}
}
