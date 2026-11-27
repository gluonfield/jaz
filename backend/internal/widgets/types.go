package widgets

import (
	"time"
)

const (
	PlacedByLLM  = "llm"
	PlacedByUser = "user"
)

const (
	DefaultGridCols  = 6
	DefaultRowHeight = 120
	MaxHTMLBytes     = 1 << 20
)

// Widget is a routine's tile on its boards. The tile shows the app the routine
// publishes; Jaz keeps the size, the board's feedback and the run that last
// published here.
type Widget struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	LoopID      string `json:"loop_id"`
	Title       string `json:"title"`
	// CurrentVersion counts HTML published before tiles became apps. It is
	// zero once the routine has published its app.
	CurrentVersion int    `json:"current_version"`
	SizeHint       string `json:"size_hint,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	// LastLayout holds the board's most recent layout telemetry for the
	// current version (JSON from the bridge); cleared on publish.
	LastLayout     string    `json:"last_layout,omitempty"`
	PublishedRunID string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Version is HTML a routine published before tiles became apps.
type Version struct {
	WidgetID  string    `json:"widget_id"`
	Version   int       `json:"version"`
	HTML      string    `json:"html"`
	CreatedAt time.Time `json:"created_at"`
}

type Board struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Name        string `json:"name"`
	GridCols    int    `json:"grid_cols"`
	RowHeight   int    `json:"row_height"`
	// FontScale zooms every widget document on the board (big-screen comfort).
	FontScale    float64   `json:"font_scale"`
	WindowBounds string    `json:"window_bounds,omitempty"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Placement struct {
	BoardID   string    `json:"board_id"`
	WidgetID  string    `json:"widget_id"`
	X         int       `json:"x"`
	Y         int       `json:"y"`
	W         int       `json:"w"`
	H         int       `json:"h"`
	PlacedBy  string    `json:"placed_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BoardItem struct {
	Placement
	LoopID            string    `json:"loop_id"`
	LoopName          string    `json:"loop_name"`
	LoopStatus        string    `json:"loop_status"`
	LoopLastRunStatus string    `json:"loop_last_run_status,omitempty"`
	LoopLastRunAt     time.Time `json:"loop_last_run_at,omitzero"`
	AppID             string    `json:"app_id,omitempty"`
	AppKind           string    `json:"app_kind,omitempty"`
	Title             string    `json:"title"`
	CurrentVersion    int       `json:"current_version"`
	SizeHint          string    `json:"size_hint,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	WidgetUpdatedAt   time.Time `json:"widget_updated_at"`
}

type Repository interface {
	NewWidgetID() string
	NewBoardID() string
	LoadWidget(id string) (Widget, error)
	LoadWidgetByLoop(loopID string) (Widget, bool, error)
	SaveWidget(widget Widget) error
	LoadWidgetVersion(widgetID string, version int) (Version, error)
	DeleteWidgetVersions(widgetID string) error
	SaveBoard(board Board) error
	LoadBoard(id string) (Board, error)
	DefaultBoard() (Board, bool, error)
	ListBoards() ([]Board, error)
	DeleteBoard(id string) error
	SavePlacement(placement Placement) error
	LoadPlacement(boardID, widgetID string) (Placement, bool, error)
	DeletePlacement(boardID, widgetID string) error
	ListPlacements(boardID string) ([]Placement, error)
	ListBoardsForWidget(widgetID string) ([]string, error)
	ListBoardItems(boardID string) ([]BoardItem, error)
	PurgeOrphanWidgets() (int, error)
}
