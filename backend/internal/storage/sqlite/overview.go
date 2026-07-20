package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) LoadSessionOverview(ctx context.Context, ref string) (storage.SessionOverview, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.SessionOverview{}, fmt.Errorf("session id or slug is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storage.SessionOverview{}, err
	}
	defer tx.Rollback()

	threads := threaddb.New(tx)
	parent, err := threads.GetSession(ctx, ref)
	if err == sql.ErrNoRows {
		return storage.SessionOverview{}, fmt.Errorf("%w: %s", storage.ErrSessionNotFound, ref)
	}
	if err != nil {
		return storage.SessionOverview{}, err
	}
	childRows, err := threads.ListOverviewChildren(ctx, nullDBString(parent.ID))
	if err != nil {
		return storage.SessionOverview{}, err
	}
	view := storage.SessionOverview{Threads: make([]storage.OverviewThread, 0, len(childRows))}
	for _, row := range childRows {
		view.Threads = append(view.Threads, storage.OverviewThread{
			ID: row.ID, Slug: row.Slug, Title: row.Title.String, Status: row.Status,
			ACPAgent: row.AcpAgent.String, Model: row.Model.String,
			ReasoningEffort: row.ReasoningEffort.String, Archived: row.Archived != 0,
			UpdatedAt: msToTime(row.UpdatedAtMs),
		})
	}

	eventRows, err := eventdb.New(tx).ListProviderSubagentEvents(ctx, parent.ID)
	if err != nil {
		return storage.SessionOverview{}, err
	}
	view.SubagentEvents = make([]sessionevents.Event, 0, len(eventRows))
	for _, row := range eventRows {
		event, decodeErr := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
		if decodeErr != nil {
			return storage.SessionOverview{}, decodeErr
		}
		view.SubagentEvents = append(view.SubagentEvents, event)
	}
	if err := tx.Commit(); err != nil {
		return storage.SessionOverview{}, err
	}
	return view, nil
}
