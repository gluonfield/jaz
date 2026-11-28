package sqlite

import (
	"context"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
)

func (s *Store) ListSessionPluginViews() ([]storage.SessionPluginView, error) {
	rows, err := eventdb.New(s.db).ListSessionPluginViews(context.Background())
	if err != nil {
		return nil, err
	}
	views := make([]storage.SessionPluginView, 0, len(rows))
	for _, row := range rows {
		event := sessionevents.Event{Type: sessionevents.TypeArtifact, Content: row.Content}
		event.NormalizePayload()
		if event.Artifact == nil || event.Artifact.Kind != sessionevents.PresentationPluginView {
			continue
		}
		views = append(views, storage.SessionPluginView{ID: row.ID, WorkspaceID: firstNonEmpty(row.WorkspaceID, storage.LocalWorkspaceID), Title: row.Title.String, Status: row.Status, UpdatedAt: msToTime(row.UpdatedAtMs), Folder: row.Cwd.String, Seq: row.Seq, Reference: event.Artifact.View.Reference, File: event.Artifact.View.File})
	}
	return views, nil
}
