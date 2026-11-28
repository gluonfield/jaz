package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	boardsdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/boards"
)

func (s *Store) BoardFontScale(ctx context.Context, boardID string) (float64, error) {
	scale, err := boardsdb.New(s.db).GetBoardFontScale(ctx, boardID)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	return scale, err
}

func (s *Store) SetBoardFontScale(ctx context.Context, boardID string, scale float64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).UpsertBoardFontScale(ctx, boardsdb.UpsertBoardFontScaleParams{BoardID: boardID, FontScale: scale})
}

func (s *Store) ListBoardTiles(ctx context.Context, boardID string) ([]storage.BoardTile, error) {
	rows, err := boardsdb.New(s.db).ListBoardTiles(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]storage.BoardTile, 0, len(rows))
	for _, r := range rows {
		tile := storage.BoardTile{
			App: appFromRow(
				r.ID, r.WorkspaceID, r.SourceThreadID, r.Title, r.Description,
				r.IconJson, r.Kind, r.Visibility, r.CurrentVersion, r.Status,
				r.CreatedAtMs, r.UpdatedAtMs, r.LoopID, r.BundleSha256, r.EntryPath,
				r.CapabilitiesJson, r.AllowedToolsJson,
			),
			X:         int(r.X),
			Y:         int(r.Y),
			W:         int(r.W),
			H:         int(r.H),
			PlacedBy:  r.PlacedBy,
			LastError: r.LastError,
		}
		if r.LoopID != "" {
			tile.Loop = &storage.TileLoop{
				ID:            r.LoopID,
				Name:          r.LoopName,
				Status:        r.LoopStatus,
				LastRunStatus: r.LoopLastRunStatus,
				LastRunAt:     msToTime(r.LoopLastRunAtMs),
			}
		}
		out = append(out, tile)
	}
	return out, nil
}

func (s *Store) PlaceBoardTiles(ctx context.Context, boardID string, placements []storage.TilePlacement) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := boardsdb.New(tx)
	nowMs := timeToMs(time.Now().UTC())
	for _, p := range placements {
		if err := q.UpsertBoardTile(ctx, boardsdb.UpsertBoardTileParams{
			BoardID:  boardID,
			AppID:    p.AppID,
			X:        int64(p.X),
			Y:        int64(p.Y),
			W:        int64(p.W),
			H:        int64(p.H),
			PlacedBy: p.PlacedBy,
			NowMs:    nowMs,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RemoveBoardTile(ctx context.Context, boardID, appID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).DeleteBoardTile(ctx, boardsdb.DeleteBoardTileParams{BoardID: boardID, AppID: appID})
}

func (s *Store) ListAppBoards(ctx context.Context, appID string) ([]string, error) {
	return boardsdb.New(s.db).ListAppBoards(ctx, appID)
}

func (s *Store) ResizeLLMTiles(ctx context.Context, appID string, w, h int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).ResizeLLMTiles(ctx, boardsdb.ResizeLLMTilesParams{
		W:     int64(w),
		H:     int64(h),
		NowMs: timeToMs(time.Now().UTC()),
		AppID: appID,
	})
}

func (s *Store) GetLoopApp(ctx context.Context, loopID string) (storage.LoopApp, error) {
	row, err := boardsdb.New(s.db).GetLoopApp(ctx, loopID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.LoopApp{}, storage.ErrAppNotFound
	}
	return storage.LoopApp(row), err
}

func (s *Store) UpdateLoopApp(ctx context.Context, app storage.LoopApp) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).UpdateLoopApp(ctx, boardsdb.UpdateLoopAppParams{
		SizeHint:       app.SizeHint,
		LastError:      app.LastError,
		LastLayout:     app.LastLayout,
		PublishedRunID: app.PublishedRunID,
		LoopID:         app.LoopID,
	})
}

func (s *Store) SetLoopAppError(ctx context.Context, appID, message string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).SetLoopAppError(ctx, boardsdb.SetLoopAppErrorParams{LastError: message, AppID: appID})
}

func (s *Store) SetLoopAppLayout(ctx context.Context, appID, layout string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).SetLoopAppLayout(ctx, boardsdb.SetLoopAppLayoutParams{LastLayout: layout, AppID: appID})
}

func (s *Store) PurgeOrphanLoopApps(ctx context.Context) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := boardsdb.New(tx)
	if err := q.DeleteOrphanLoopAppTiles(ctx); err != nil {
		return 0, err
	}
	if err := q.DeleteUnpublishedOrphanLoopApps(ctx); err != nil {
		return 0, err
	}
	removed, err := q.DeleteOrphanLoopApps(ctx)
	if err != nil {
		return 0, err
	}
	return int(removed), tx.Commit()
}

func (s *Store) ListLegacyWidgetHTML(ctx context.Context) ([]storage.LegacyWidgetHTML, error) {
	rows, err := boardsdb.New(s.db).ListLegacyWidgetHTML(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.LegacyWidgetHTML, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.LegacyWidgetHTML{AppID: row.AppID, Title: row.Title, HTML: row.Html})
	}
	return out, nil
}

func (s *Store) DeleteLegacyWidgetHTML(ctx context.Context, appID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return boardsdb.New(s.db).DeleteLegacyWidgetHTML(ctx, appID)
}
