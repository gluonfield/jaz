package sqlite

import (
	"context"
	"database/sql"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/widgetdb"
	"github.com/wins/jaz/backend/internal/widgets"
)

func (s *Store) NewWidgetID() string {
	return "widget-" + s.NewSessionID()
}

// PurgeOrphanWidgets removes widgets (and their versions and placements)
// whose loop is gone or soft-deleted. Orphan placements are invisible on the
// board but still occupy grid cells, blocking placement and drags.
func (s *Store) PurgeOrphanWidgets() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	q := widgetdb.New(s.db)
	deleted := string(loops.StatusDeleted)
	if err := q.DeleteOrphanBoardWidgets(ctx, deleted); err != nil {
		return 0, err
	}
	if err := q.DeleteOrphanWidgetVersions(ctx, deleted); err != nil {
		return 0, err
	}
	removed, err := q.DeleteOrphanWidgets(ctx, deleted)
	return int(removed), err
}

func (s *Store) NewBoardID() string {
	return "board-" + s.NewSessionID()
}

func (s *Store) LoadWidget(id string) (widgets.Widget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetWidget(context.Background(), id)
	if err != nil {
		return widgets.Widget{}, err
	}
	return widgetFromDB(row), nil
}

func (s *Store) LoadWidgetByLoop(loopID string) (widgets.Widget, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetWidgetByLoop(context.Background(), loopID)
	if err == sql.ErrNoRows {
		return widgets.Widget{}, false, nil
	}
	if err != nil {
		return widgets.Widget{}, false, err
	}
	return widgetFromDB(row), true, nil
}

func (s *Store) ListWidgets() ([]widgets.Widget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := widgetdb.New(s.db).ListWidgets(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]widgets.Widget, 0, len(rows))
	for _, row := range rows {
		out = append(out, widgetFromDB(row))
	}
	return out, nil
}

func (s *Store) SaveWidget(widget widgets.Widget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).UpsertWidget(context.Background(), upsertWidgetParams(widget))
}

func upsertWidgetParams(widget widgets.Widget) widgetdb.UpsertWidgetParams {
	return widgetdb.UpsertWidgetParams{
		ID:             widget.ID,
		LoopID:         widget.LoopID,
		Title:          widget.Title,
		CurrentVersion: int64(widget.CurrentVersion),
		SizeHint:       widget.SizeHint,
		LastError:      nullDBString(widget.LastError),
		LastLayout:     widget.LastLayout,
		CreatedAtMs:    timeToMs(widget.CreatedAt),
		UpdatedAtMs:    timeToMs(widget.UpdatedAt),
	}
}

func (s *Store) InsertWidgetVersion(version widgets.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).InsertWidgetVersion(context.Background(), insertWidgetVersionParams(version))
}

// SaveWidgetWithVersion persists the bumped widget row and its new version
// snapshot in one transaction: a widget must never point at a version that
// was not written.
func (s *Store) SaveWidgetWithVersion(widget widgets.Widget, version widgets.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := widgetdb.New(tx)
	if err := q.UpsertWidget(ctx, upsertWidgetParams(widget)); err != nil {
		return err
	}
	if err := q.InsertWidgetVersion(ctx, insertWidgetVersionParams(version)); err != nil {
		return err
	}
	return tx.Commit()
}

func insertWidgetVersionParams(version widgets.Version) widgetdb.InsertWidgetVersionParams {
	return widgetdb.InsertWidgetVersionParams{
		WidgetID:        version.WidgetID,
		Version:         int64(version.Version),
		Html:            version.HTML,
		ProducedByRunID: nullDBString(version.ProducedByRunID),
		CreatedAtMs:     timeToMs(version.CreatedAt),
	}
}

func (s *Store) LoadWidgetVersion(widgetID string, version int) (widgets.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetWidgetVersion(context.Background(), widgetdb.GetWidgetVersionParams{
		WidgetID: widgetID,
		Version:  int64(version),
	})
	if err != nil {
		return widgets.Version{}, err
	}
	return widgets.Version{
		WidgetID:        row.WidgetID,
		Version:         int(row.Version),
		HTML:            row.Html,
		ProducedByRunID: row.ProducedByRunID.String,
		CreatedAt:       msToTime(row.CreatedAtMs),
	}, nil
}

func (s *Store) PruneOldWidgetVersions(widgetID string, currentVersion, keepOld int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).PruneWidgetVersions(context.Background(), widgetdb.PruneWidgetVersionsParams{
		WidgetID:       widgetID,
		MaxKeepVersion: int64(currentVersion - keepOld - 1),
	})
}

func (s *Store) SaveBoard(board widgets.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).UpsertBoard(context.Background(), widgetdb.UpsertBoardParams{
		ID:           board.ID,
		Name:         board.Name,
		GridCols:     int64(board.GridCols),
		RowHeight:    int64(board.RowHeight),
		FontScale:    board.FontScale,
		WindowBounds: nullDBString(board.WindowBounds),
		IsDefault:    boolInt(board.IsDefault),
		CreatedAtMs:  timeToMs(board.CreatedAt),
		UpdatedAtMs:  timeToMs(board.UpdatedAt),
	})
}

func (s *Store) LoadBoard(id string) (widgets.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetBoard(context.Background(), id)
	if err != nil {
		return widgets.Board{}, err
	}
	return boardFromDB(row), nil
}

func (s *Store) DefaultBoard() (widgets.Board, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetDefaultBoard(context.Background())
	if err == sql.ErrNoRows {
		return widgets.Board{}, false, nil
	}
	if err != nil {
		return widgets.Board{}, false, err
	}
	return boardFromDB(row), true, nil
}

func (s *Store) ListBoards() ([]widgets.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := widgetdb.New(s.db).ListBoards(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]widgets.Board, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardFromDB(row))
	}
	return out, nil
}

func (s *Store) DeleteBoard(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).DeleteBoard(context.Background(), id)
}

func (s *Store) SavePlacement(placement widgets.Placement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).UpsertBoardWidget(context.Background(), widgetdb.UpsertBoardWidgetParams{
		BoardID:     placement.BoardID,
		WidgetID:    placement.WidgetID,
		X:           int64(placement.X),
		Y:           int64(placement.Y),
		W:           int64(placement.W),
		H:           int64(placement.H),
		PlacedBy:    placement.PlacedBy,
		CreatedAtMs: timeToMs(placement.CreatedAt),
		UpdatedAtMs: timeToMs(placement.UpdatedAt),
	})
}

func (s *Store) LoadPlacement(boardID, widgetID string) (widgets.Placement, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := widgetdb.New(s.db).GetBoardWidget(context.Background(), widgetdb.GetBoardWidgetParams{
		BoardID:  boardID,
		WidgetID: widgetID,
	})
	if err == sql.ErrNoRows {
		return widgets.Placement{}, false, nil
	}
	if err != nil {
		return widgets.Placement{}, false, err
	}
	return placementFromDB(row), true, nil
}

func (s *Store) DeletePlacement(boardID, widgetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).DeleteBoardWidget(context.Background(), widgetdb.DeleteBoardWidgetParams{
		BoardID:  boardID,
		WidgetID: widgetID,
	})
}

func (s *Store) ListBoardsForWidget(widgetID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return widgetdb.New(s.db).ListBoardsForWidget(context.Background(), widgetID)
}

func (s *Store) ListPlacements(boardID string) ([]widgets.Placement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := widgetdb.New(s.db).ListAllPlacements(context.Background(), boardID)
	if err != nil {
		return nil, err
	}
	out := make([]widgets.Placement, 0, len(rows))
	for _, row := range rows {
		out = append(out, placementFromDB(widgetdb.BoardWidget(row)))
	}
	return out, nil
}

func (s *Store) ListBoardItems(boardID string) ([]widgets.BoardItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := widgetdb.New(s.db).ListBoardItems(context.Background(), widgetdb.ListBoardItemsParams{
		BoardID:       boardID,
		DeletedStatus: loops.StatusDeleted,
	})
	if err != nil {
		return nil, err
	}
	out := make([]widgets.BoardItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, widgets.BoardItem{
			Placement: widgets.Placement{
				BoardID:  row.BoardID,
				WidgetID: row.WidgetID,
				X:        int(row.X),
				Y:        int(row.Y),
				W:        int(row.W),
				H:        int(row.H),
				PlacedBy: row.PlacedBy,
			},
			LoopID:            row.LoopID,
			LoopName:          row.LoopName,
			LoopStatus:        row.LoopStatus,
			LoopLastRunStatus: row.LoopLastRunStatus.String,
			LoopLastRunAt:     msToTime(row.LoopLastRunAtMs),
			Title:             row.Title,
			CurrentVersion:    int(row.CurrentVersion),
			SizeHint:          row.SizeHint,
			LastError:         row.LastError.String,
			WidgetUpdatedAt:   msToTime(row.WidgetUpdatedAtMs),
		})
	}
	return out, nil
}

func widgetFromDB(row widgetdb.Widget) widgets.Widget {
	return widgets.Widget{
		ID:             row.ID,
		LoopID:         row.LoopID,
		Title:          row.Title,
		CurrentVersion: int(row.CurrentVersion),
		SizeHint:       row.SizeHint,
		LastError:      row.LastError.String,
		LastLayout:     row.LastLayout,
		CreatedAt:      msToTime(row.CreatedAtMs),
		UpdatedAt:      msToTime(row.UpdatedAtMs),
	}
}

func boardFromDB(row widgetdb.Board) widgets.Board {
	return widgets.Board{
		ID:           row.ID,
		Name:         row.Name,
		GridCols:     int(row.GridCols),
		RowHeight:    int(row.RowHeight),
		FontScale:    row.FontScale,
		WindowBounds: row.WindowBounds.String,
		IsDefault:    row.IsDefault != 0,
		CreatedAt:    msToTime(row.CreatedAtMs),
		UpdatedAt:    msToTime(row.UpdatedAtMs),
	}
}

func placementFromDB(row widgetdb.BoardWidget) widgets.Placement {
	return widgets.Placement{
		BoardID:   row.BoardID,
		WidgetID:  row.WidgetID,
		X:         int(row.X),
		Y:         int(row.Y),
		W:         int(row.W),
		H:         int(row.H),
		PlacedBy:  row.PlacedBy,
		CreatedAt: msToTime(row.CreatedAtMs),
		UpdatedAt: msToTime(row.UpdatedAtMs),
	}
}
