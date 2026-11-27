package sqlite

import (
	"context"
	"database/sql"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/widgetdb"
	"github.com/wins/jaz/backend/internal/widgets"
)

func (s *Store) NewWidgetID() string {
	return "widget-" + s.NewSessionID()
}

// PurgeOrphanWidgets removes widgets (and their versions and placements)
// whose loop is gone or soft-deleted. Orphan placements are invisible on the
// board but still occupy grid cells, blocking placement and drags. The app
// such a loop published stays in the library as an ordinary app.
func (s *Store) PurgeOrphanWidgets() (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	q := widgetdb.New(s.db)
	deleted := string(loops.StatusDeleted)
	if err := q.DeleteOrphanBoardWidgets(ctx, deleted); err != nil {
		return 0, err
	}
	if err := q.DeleteOrphanWidgetVersions(ctx, deleted); err != nil {
		return 0, err
	}
	if err := q.ReleaseOrphanLoopApps(ctx, deleted); err != nil {
		return 0, err
	}
	removed, err := q.DeleteOrphanWidgets(ctx, deleted)
	return int(removed), err
}

func (s *Store) NewBoardID() string {
	return "board-" + s.NewSessionID()
}

func (s *Store) LoadWidget(id string) (widgets.Widget, error) {
	row, err := widgetdb.New(s.db).GetWidget(context.Background(), id)
	if err != nil {
		return widgets.Widget{}, err
	}
	return widgetFromDB(row), nil
}

func (s *Store) LoadWidgetByLoop(loopID string) (widgets.Widget, bool, error) {
	row, err := widgetdb.New(s.db).GetWidgetByLoop(context.Background(), loopID)
	if err == sql.ErrNoRows {
		return widgets.Widget{}, false, nil
	}
	if err != nil {
		return widgets.Widget{}, false, err
	}
	return widgetFromDB(row), true, nil
}

func (s *Store) SaveWidget(widget widgets.Widget) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return widgetdb.New(s.db).UpsertWidget(context.Background(), upsertWidgetParams(widget))
}

func upsertWidgetParams(widget widgets.Widget) widgetdb.UpsertWidgetParams {
	return widgetdb.UpsertWidgetParams{
		ID:             widget.ID,
		LoopID:         widget.LoopID,
		Title:          widget.Title,
		SizeHint:       widget.SizeHint,
		LastError:      nullDBString(widget.LastError),
		LastLayout:     widget.LastLayout,
		CreatedAtMs:    timeToMs(widget.CreatedAt),
		UpdatedAtMs:    timeToMs(widget.UpdatedAt),
		WorkspaceID:    firstNonEmpty(widget.WorkspaceID, storage.LocalWorkspaceID),
		PublishedRunID: widget.PublishedRunID,
	}
}

func (s *Store) ListLegacyWidgetHTML() ([]widgets.LegacyHTML, error) {
	rows, err := widgetdb.New(s.db).ListLegacyWidgetHTML(context.Background(), string(loops.StatusDeleted))
	if err != nil {
		return nil, err
	}
	out := make([]widgets.LegacyHTML, 0, len(rows))
	for _, row := range rows {
		out = append(out, widgets.LegacyHTML{WidgetID: row.ID, LoopID: row.LoopID, Title: row.Title, HTML: row.Html})
	}
	return out, nil
}

func (s *Store) DeleteWidgetVersions(widgetID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return widgetdb.New(s.db).DeleteWidgetVersions(context.Background(), widgetID)
}

func (s *Store) SaveBoard(board widgets.Board) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
		WorkspaceID:  firstNonEmpty(board.WorkspaceID, storage.LocalWorkspaceID),
	})
}

func (s *Store) LoadBoard(id string) (widgets.Board, error) {
	row, err := widgetdb.New(s.db).GetBoard(context.Background(), id)
	if err != nil {
		return widgets.Board{}, err
	}
	return boardFromDB(row), nil
}

func (s *Store) DefaultBoard() (widgets.Board, bool, error) {
	return s.DefaultBoardForWorkspace(storage.LocalWorkspaceID)
}

func (s *Store) DefaultBoardForWorkspace(workspaceID string) (widgets.Board, bool, error) {
	row, err := widgetdb.New(s.db).GetDefaultBoard(context.Background(), firstNonEmpty(workspaceID, storage.LocalWorkspaceID))
	if err == sql.ErrNoRows {
		return widgets.Board{}, false, nil
	}
	if err != nil {
		return widgets.Board{}, false, err
	}
	return boardFromDB(row), true, nil
}

func (s *Store) ListBoards() ([]widgets.Board, error) {
	return s.ListBoardsForWorkspace("")
}

func (s *Store) ListBoardsForWorkspace(workspaceID string) ([]widgets.Board, error) {
	rows, err := widgetdb.New(s.db).ListBoards(context.Background(), workspaceID)
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return widgetdb.New(s.db).DeleteBoard(context.Background(), id)
}

func (s *Store) SavePlacement(placement widgets.Placement) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
	}, true, nil
}

func (s *Store) DeletePlacement(boardID, widgetID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return widgetdb.New(s.db).DeleteBoardWidget(context.Background(), widgetdb.DeleteBoardWidgetParams{
		BoardID:  boardID,
		WidgetID: widgetID,
	})
}

func (s *Store) ListBoardsForWidget(widgetID string) ([]string, error) {
	return widgetdb.New(s.db).ListBoardsForWidget(context.Background(), widgetID)
}

func (s *Store) ListPlacements(boardID string) ([]widgets.Placement, error) {
	rows, err := widgetdb.New(s.db).ListAllPlacements(context.Background(), boardID)
	if err != nil {
		return nil, err
	}
	out := make([]widgets.Placement, 0, len(rows))
	for _, row := range rows {
		out = append(out, widgets.Placement{
			BoardID:   row.BoardID,
			WidgetID:  row.WidgetID,
			X:         int(row.X),
			Y:         int(row.Y),
			W:         int(row.W),
			H:         int(row.H),
			PlacedBy:  row.PlacedBy,
			CreatedAt: msToTime(row.CreatedAtMs),
			UpdatedAt: msToTime(row.UpdatedAtMs),
		})
	}
	return out, nil
}

func (s *Store) ListBoardItems(boardID string) ([]widgets.BoardItem, error) {
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
			AppID:             row.AppID,
			AppKind:           row.AppKind,
			LoopID:            row.LoopID,
			LoopName:          row.LoopName,
			LoopStatus:        row.LoopStatus,
			LoopLastRunStatus: row.LoopLastRunStatus,
			LoopLastRunAt:     msToTime(row.LoopLastRunAtMs),
			Title:             row.Title,
			CurrentVersion:    int(row.CurrentVersion),
			SizeHint:          row.SizeHint,
			LastError:         row.LastError,
			WidgetUpdatedAt:   msToTime(row.UpdatedAtMs),
		})
	}
	return out, nil
}

func widgetFromDB(row widgetdb.Widget) widgets.Widget {
	return widgets.Widget{
		ID:             row.ID,
		WorkspaceID:    firstNonEmpty(row.WorkspaceID, storage.LocalWorkspaceID),
		LoopID:         row.LoopID,
		Title:          row.Title,
		SizeHint:       row.SizeHint,
		LastError:      row.LastError.String,
		LastLayout:     row.LastLayout,
		PublishedRunID: row.PublishedRunID,
		CreatedAt:      msToTime(row.CreatedAtMs),
		UpdatedAt:      msToTime(row.UpdatedAtMs),
	}
}

func boardFromDB(row widgetdb.Board) widgets.Board {
	return widgets.Board{
		ID:           row.ID,
		WorkspaceID:  firstNonEmpty(row.WorkspaceID, storage.LocalWorkspaceID),
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
