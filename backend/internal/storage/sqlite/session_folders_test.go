package sqlite

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestSessionFoldersIncludesArchivedAndSourcedChildren(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	folder := t.TempDir()
	root, err := store.CreateSession(storage.CreateSession{Title: "Root", RuntimeRef: &storage.RuntimeRef{Cwd: folder}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Title: "Sourced child", ParentID: root.ID, SourceType: "fixture", SourceID: "source", RuntimeRef: &storage.RuntimeRef{Cwd: folder}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(child.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(storage.CreateSession{Title: "No folder"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(storage.CreateSession{Title: "Empty folder", RuntimeRef: &storage.RuntimeRef{}}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListSessionFolders()
	if err != nil || len(rows) != 2 {
		t.Fatalf("folders = %+v, %v", rows, err)
	}
	for _, row := range rows {
		want := root
		if row.ID == child.ID {
			want = child
		}
		if row.ID != want.ID || row.Title != want.Title || row.Status != want.Status || row.Folder != folder || !row.UpdatedAt.Equal(want.UpdatedAt.Truncate(time.Millisecond)) {
			t.Fatalf("folder = %+v, want session %+v", row, want)
		}
	}
}
