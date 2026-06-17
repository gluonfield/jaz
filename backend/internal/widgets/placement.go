package widgets

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
)

// Boards, placements, and the tile-grid geometry: assignment is the widget
// enablement, the LLM proposes sizes, the user's layout always wins, and
// tiles never overlap.

// resolveBoardsLocked validates every board ID up front (deduplicated, in
// order) so callers can fail before any write happens.
func (s *Service) resolveBoardsLocked(boardIDs []string) ([]Board, error) {
	seen := make(map[string]bool, len(boardIDs))
	boards := make([]Board, 0, len(boardIDs))
	for _, boardID := range boardIDs {
		boardID = strings.TrimSpace(boardID)
		if boardID == "" || seen[boardID] {
			continue
		}
		board, err := s.Repo.LoadBoard(boardID)
		if err != nil {
			return nil, fmt.Errorf("unknown board %s", boardID)
		}
		seen[board.ID] = true
		boards = append(boards, board)
	}
	return boards, nil
}

// ValidateBoardIDs lets callers (e.g. loop create/patch) reject bad board ids
// before persisting anything else.
func (s *Service) ValidateBoardIDs(boardIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.resolveBoardsLocked(boardIDs)
	return err
}

// AssignLoopBoards makes board assignment the single source of widget
// enablement: the widget row is created eagerly (version 0 renders as a
// waiting tile) and placements are reconciled to exactly the given boards.
// All boards are validated before anything is written.
func (s *Service) AssignLoopBoards(loop loops.Loop, boardIDs []string) (Widget, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	boards, err := s.resolveBoardsLocked(boardIDs)
	if err != nil {
		return Widget{}, err
	}
	widget, found, err := s.Repo.LoadWidgetByLoop(loop.ID)
	if err != nil {
		return Widget{}, err
	}
	if !found {
		widget = Widget{
			ID:        s.Repo.NewWidgetID(),
			LoopID:    loop.ID,
			Title:     loop.Name,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.Repo.SaveWidget(widget); err != nil {
			return Widget{}, err
		}
	}
	wanted := make(map[string]bool, len(boards))
	for _, board := range boards {
		wanted[board.ID] = true
		if _, found, err := s.Repo.LoadPlacement(board.ID, widget.ID); err != nil {
			return Widget{}, err
		} else if found {
			continue
		}
		w, h := parseSizeHint(widget.SizeHint)
		existing, err := s.Repo.ListPlacements(board.ID)
		if err != nil {
			return Widget{}, err
		}
		x, y := firstFreeSpot(existing, board.GridCols, w, h)
		if err := s.Repo.SavePlacement(Placement{
			BoardID:   board.ID,
			WidgetID:  widget.ID,
			X:         x,
			Y:         y,
			W:         w,
			H:         h,
			PlacedBy:  PlacedByLLM,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			return Widget{}, err
		}
	}
	current, err := s.Repo.ListBoardsForWidget(widget.ID)
	if err != nil {
		return Widget{}, err
	}
	for _, boardID := range current {
		if !wanted[boardID] {
			if err := s.Repo.DeletePlacement(boardID, widget.ID); err != nil {
				return Widget{}, err
			}
		}
	}
	return widget, nil
}

// AddLoopToBoard assigns additively (the onboarding flow adds loops to a
// fresh board without touching their other assignments).
func (s *Service) AddLoopToBoard(loop loops.Loop, boardID string) (Widget, error) {
	_, boards, found, err := s.StateForLoop(loop.ID)
	if err != nil {
		return Widget{}, err
	}
	if !found {
		boards = nil
	}
	return s.AssignLoopBoards(loop, append(boards, boardID))
}

// applySizeHintLocked resizes placements the LLM still owns to the published
// size hint. User-placed tiles are never touched.
func (s *Service) applySizeHintLocked(widget Widget, boardIDs []string, now time.Time) {
	if widget.SizeHint == "" {
		return
	}
	w, h := parseSizeHint(widget.SizeHint)
	for _, boardID := range boardIDs {
		placement, found, err := s.Repo.LoadPlacement(boardID, widget.ID)
		if err != nil || !found || placement.PlacedBy != PlacedByLLM {
			continue
		}
		if placement.W == w && placement.H == h {
			continue
		}
		placement.W, placement.H = w, h
		placement.UpdatedAt = now
		if err := s.Repo.SavePlacement(placement); err != nil {
			s.Log.Warn("resizing widget placement failed", "widget", widget.ID, "board", boardID, "error", err)
		}
	}
}

func (s *Service) CreateBoard(name string) (Board, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Board{}, fmt.Errorf("board name is required")
	}
	now := s.now()
	board := Board{
		ID:        s.Repo.NewBoardID(),
		Name:      name,
		GridCols:  DefaultGridCols,
		RowHeight: DefaultRowHeight,
		FontScale: 1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return board, s.Repo.SaveBoard(board)
}

type UpdateBoard struct {
	Name         *string
	WindowBounds *string
	FontScale    *float64
	Layout       []LayoutEntry
}

type LayoutEntry struct {
	WidgetID string `json:"widget_id"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
}

func (s *Service) PatchBoard(id string, input UpdateBoard) (Board, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.Repo.LoadBoard(id)
	if err != nil {
		return Board{}, err
	}
	// Validate the whole request before writing anything: a rejected patch
	// must not leave half its changes applied.
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Board{}, fmt.Errorf("board name is required")
	}
	if len(input.Layout) > 0 {
		// Tiles never overlap: a stacked tile is invisible, and every hidden
		// tile pushes firstFreeSpot further below the fold.
		existing, err := s.Repo.ListPlacements(board.ID)
		if err != nil {
			return Board{}, err
		}
		moved := make(map[string]bool, len(input.Layout))
		for _, entry := range input.Layout {
			moved[entry.WidgetID] = true
		}
		var obstacles []Placement
		for _, p := range existing {
			if !moved[p.WidgetID] {
				obstacles = append(obstacles, p)
			}
		}
		for _, entry := range input.Layout {
			x, y := max(0, entry.X), max(0, entry.Y)
			w := clamp(entry.W, 1, board.GridCols)
			h := clamp(entry.H, 1, 12)
			if overlapsAny(obstacles, x, y, w, h) {
				return Board{}, fmt.Errorf("tiles cannot overlap")
			}
			obstacles = append(obstacles, Placement{X: x, Y: y, W: w, H: h})
		}
	}
	if input.Name != nil {
		board.Name = strings.TrimSpace(*input.Name)
	}
	if input.WindowBounds != nil {
		board.WindowBounds = *input.WindowBounds
	}
	if input.FontScale != nil {
		board.FontScale = clampScale(*input.FontScale)
	}
	board.UpdatedAt = now
	if err := s.Repo.SaveBoard(board); err != nil {
		return Board{}, err
	}
	for _, entry := range input.Layout {
		placement, found, err := s.Repo.LoadPlacement(board.ID, entry.WidgetID)
		if err != nil {
			return Board{}, err
		}
		if !found {
			placement = Placement{BoardID: board.ID, WidgetID: entry.WidgetID, CreatedAt: now}
		}
		placement.X = max(0, entry.X)
		placement.Y = max(0, entry.Y)
		placement.W = clamp(entry.W, 1, board.GridCols)
		placement.H = clamp(entry.H, 1, 12)
		placement.PlacedBy = PlacedByUser
		placement.UpdatedAt = now
		if err := s.Repo.SavePlacement(placement); err != nil {
			return Board{}, err
		}
	}
	return board, nil
}

// PurgeOrphans drops widgets whose loop is deleted or missing. Their
// placements are invisible on the board yet still occupy cells — blocking
// new-widget placement, drags, and resizes.
func (s *Service) PurgeOrphans() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeOrphansLocked()
}

func (s *Service) purgeOrphansLocked() {
	removed, err := s.Repo.PurgeOrphanWidgets()
	if err != nil {
		s.Log.Warn("purging orphan widgets failed", "error", err)
		return
	}
	if removed > 0 {
		s.Log.Info("purged orphan widgets of deleted loops", "count", removed)
	}
}

func (s *Service) RemoveFromBoard(boardID, widgetID string) error {
	return s.Repo.DeletePlacement(boardID, widgetID)
}

// DeleteBoard removes the board and its placements only. Widgets and their
// version history survive; loops whose widget loses its last board are simply
// disabled until reassigned.
func (s *Service) DeleteBoard(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Repo.LoadBoard(id); err != nil {
		return err
	}
	return s.Repo.DeleteBoard(id)
}

func normalizeSizeHint(hint string) string {
	w, h, ok := parseSize(hint)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func parseSizeHint(hint string) (int, int) {
	if w, h, ok := parseSize(hint); ok {
		return w, h
	}
	return 2, 2
}

func parseSize(hint string) (int, int, bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(hint)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil {
		return 0, 0, false
	}
	return clamp(w, 1, DefaultGridCols), clamp(h, 1, 8), true
}

func firstFreeSpot(existing []Placement, cols, w, h int) (int, int) {
	if cols <= 0 {
		cols = DefaultGridCols
	}
	w = clamp(w, 1, cols)
	for y := 0; ; y++ {
		for x := 0; x+w <= cols; x++ {
			if !overlapsAny(existing, x, y, w, h) {
				return x, y
			}
		}
	}
}

func overlapsAny(existing []Placement, x, y, w, h int) bool {
	for _, p := range existing {
		if x < p.X+p.W && p.X < x+w && y < p.Y+p.H && p.Y < y+h {
			return true
		}
	}
	return false
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampScale(v float64) float64 {
	if v < 0.7 {
		return 0.7
	}
	if v > 2 {
		return 2
	}
	return v
}
