package boards

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

// A loop proposes its tile's size, the user's layout always wins, and tiles
// never overlap: a stacked tile is invisible, and every hidden tile pushes
// new ones further below the fold.

const (
	maxTileHeight  = 12
	pinnedTileSize = 3
)

func (s *Service) resolveBoards(ctx context.Context, boardIDs []string, workspaceID string) ([]string, error) {
	seen := make(map[string]bool, len(boardIDs))
	resolved := make([]string, 0, len(boardIDs))
	for _, boardID := range boardIDs {
		boardID = strings.TrimSpace(boardID)
		if boardID == "" || seen[boardID] {
			continue
		}
		board, err := s.store.GetApp(ctx, boardID)
		if err != nil || board.Kind != storage.AppKindBoard || board.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("unknown board %s", boardID)
		}
		seen[boardID] = true
		resolved = append(resolved, boardID)
	}
	return resolved, nil
}

// ValidateBoardIDs lets loop create and patch reject bad board IDs before
// they persist anything else.
func (s *Service) ValidateBoardIDs(ctx context.Context, boardIDs []string) error {
	_, err := s.resolveBoards(ctx, boardIDs, s.workspace())
	return err
}

// AssignLoopBoards puts the loop's app on exactly the given boards. The app
// exists from the first assignment, so its tile waits there for the first run.
// All boards are validated before anything is written.
func (s *Service) AssignLoopBoards(ctx context.Context, loop loops.Loop, boardIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspaceID := loop.WorkspaceID
	if workspaceID == "" {
		workspaceID = s.workspace()
	}
	wanted, err := s.resolveBoards(ctx, boardIDs, workspaceID)
	if err != nil {
		return err
	}
	loopApp, current, err := s.StateForLoop(ctx, loop.ID)
	if err != nil {
		return err
	}
	if len(wanted) == 0 && len(current) == 0 {
		return nil
	}
	keep := make(map[string]bool, len(wanted))
	for _, boardID := range wanted {
		keep[boardID] = true
		if err := s.addLoopTile(ctx, boardID, loop); err != nil {
			return err
		}
	}
	for _, boardID := range current {
		if !keep[boardID] {
			if err := s.store.RemoveBoardTile(ctx, boardID, loopApp.AppID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) addLoopTile(ctx context.Context, boardID string, loop loops.Loop) error {
	app, err := s.apps.EnsureLoopApp(ctx, loop.ID, loop.Name)
	if err != nil {
		return err
	}
	loopApp, err := s.store.GetLoopApp(ctx, loop.ID)
	if err != nil {
		return err
	}
	w, h := 2, 2
	if hintW, hintH, ok := parseSize(loopApp.SizeHint); ok {
		w, h = hintW, hintH
	}
	return s.addTile(ctx, boardID, app.ID, w, h, storage.TilePlacedByLLM)
}

func (s *Service) addTile(ctx context.Context, boardID, appID string, w, h int, placedBy string) error {
	tiles, err := s.store.ListBoardTiles(ctx, boardID)
	if err != nil {
		return err
	}
	taken := make([]rect, 0, len(tiles))
	for _, tile := range tiles {
		if tile.App.ID == appID {
			return nil
		}
		taken = append(taken, rect{tile.X, tile.Y, tile.W, tile.H})
	}
	x, y := firstFreeSpot(taken, w, h)
	return s.store.PlaceBoardTiles(ctx, boardID, []storage.TilePlacement{{AppID: appID, X: x, Y: y, W: w, H: h, PlacedBy: placedBy}})
}

// AddTiles puts apps and the apps of loops on a board, keeping the tiles they
// already have there. A board cannot be a tile.
func (s *Service) AddTiles(ctx context.Context, boardID string, appIDs []string, tileLoops []loops.Loop) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.board(ctx, boardID); err != nil {
		return err
	}
	for _, appID := range appIDs {
		app, err := s.store.GetApp(ctx, appID)
		if err != nil {
			return err
		}
		if app.Kind == storage.AppKindBoard {
			return fmt.Errorf("a board cannot be a tile on a board")
		}
	}
	for _, appID := range appIDs {
		if err := s.addTile(ctx, boardID, appID, pinnedTileSize, pinnedTileSize, storage.TilePlacedByUser); err != nil {
			return err
		}
	}
	for _, loop := range tileLoops {
		if err := s.addLoopTile(ctx, boardID, loop); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RemoveTile(ctx context.Context, boardID, appID string) error {
	if _, err := s.board(ctx, boardID); err != nil {
		return err
	}
	return s.store.RemoveBoardTile(ctx, boardID, appID)
}

type Update struct {
	FontScale *float64
	Layout    []LayoutEntry
}

type LayoutEntry struct {
	AppID string `json:"app_id"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

// Patch moves and resizes tiles already on the board. The whole layout is
// checked before anything is written.
func (s *Service) Patch(ctx context.Context, boardID string, input Update) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.board(ctx, boardID); err != nil {
		return err
	}
	placements := make([]storage.TilePlacement, 0, len(input.Layout))
	if len(input.Layout) > 0 {
		tiles, err := s.store.ListBoardTiles(ctx, boardID)
		if err != nil {
			return err
		}
		moved := make(map[string]bool, len(input.Layout))
		for _, entry := range input.Layout {
			moved[entry.AppID] = true
		}
		var taken []rect
		for _, tile := range tiles {
			if !moved[tile.App.ID] {
				taken = append(taken, rect{tile.X, tile.Y, tile.W, tile.H})
			}
			delete(moved, tile.App.ID)
		}
		if len(moved) > 0 {
			return fmt.Errorf("the layout names an app that is not on this board")
		}
		for _, entry := range input.Layout {
			next := rect{max(0, entry.X), max(0, entry.Y), min(max(entry.W, 1), GridCols), min(max(entry.H, 1), maxTileHeight)}
			if next.overlapsAny(taken) {
				return fmt.Errorf("tiles cannot overlap")
			}
			taken = append(taken, next)
			placements = append(placements, storage.TilePlacement{AppID: entry.AppID, X: next.x, Y: next.y, W: next.w, H: next.h, PlacedBy: storage.TilePlacedByUser})
		}
	}
	if input.FontScale != nil {
		if err := s.store.SetBoardFontScale(ctx, boardID, min(max(*input.FontScale, 0.7), 2)); err != nil {
			return err
		}
	}
	return s.store.PlaceBoardTiles(ctx, boardID, placements)
}

// PurgeOrphans takes the apps of deleted loops off their boards, where they
// would keep occupying cells with nothing to refresh them.
func (s *Service) PurgeOrphans(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed, err := s.store.PurgeOrphanLoopApps(ctx)
	if err != nil {
		s.log.Warn("purging the apps of deleted loops failed", "error", err)
		return
	}
	if removed > 0 {
		s.log.Info("purged the apps of deleted loops", "count", removed)
	}
}

// parseSize reads a WxH size hint, clamped to what a loop may ask for.
func parseSize(hint string) (int, int, bool) {
	rawW, rawH, ok := strings.Cut(strings.ToLower(strings.TrimSpace(hint)), "x")
	if !ok {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(strings.TrimSpace(rawW))
	h, errH := strconv.Atoi(strings.TrimSpace(rawH))
	if errW != nil || errH != nil {
		return 0, 0, false
	}
	return min(max(w, 1), GridCols), min(max(h, 1), 8), true
}

type rect struct{ x, y, w, h int }

func (r rect) overlapsAny(others []rect) bool {
	for _, o := range others {
		if r.x < o.x+o.w && o.x < r.x+r.w && r.y < o.y+o.h && o.y < r.y+r.h {
			return true
		}
	}
	return false
}

func firstFreeSpot(taken []rect, w, h int) (int, int) {
	for y := 0; ; y++ {
		for x := 0; x+w <= GridCols; x++ {
			if !(rect{x, y, w, h}).overlapsAny(taken) {
				return x, y
			}
		}
	}
}
