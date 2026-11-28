package storage

import (
	"context"
	"time"
)

const (
	TilePlacedByLLM  = "llm"
	TilePlacedByUser = "user"
)

// BoardTile is an app on a board, with the loop that keeps it up to date.
type BoardTile struct {
	App       App       `json:"app"`
	X         int       `json:"x"`
	Y         int       `json:"y"`
	W         int       `json:"w"`
	H         int       `json:"h"`
	PlacedBy  string    `json:"-"`
	Loop      *TileLoop `json:"loop,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type TileLoop struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	LastRunStatus string    `json:"last_run_status,omitempty"`
	LastRunAt     time.Time `json:"last_run_at,omitzero"`
}

type TilePlacement struct {
	AppID    string
	X        int
	Y        int
	W        int
	H        int
	PlacedBy string
}

// LoopApp is the app a loop keeps on its boards, with what the loop's next run
// needs to know about it.
type LoopApp struct {
	LoopID   string
	AppID    string
	SizeHint string
	// LastError and LastLayout are what boards reported about the current
	// version; a publish clears them.
	LastError      string
	LastLayout     string
	PublishedRunID string
}

// LegacyWidgetHTML is the HTML a loop published before tiles became apps.
type LegacyWidgetHTML struct {
	AppID string
	Title string
	HTML  string
}

type BoardStore interface {
	BoardFontScale(ctx context.Context, boardID string) (float64, error)
	SetBoardFontScale(ctx context.Context, boardID string, scale float64) error
	ListBoardTiles(ctx context.Context, boardID string) ([]BoardTile, error)
	PlaceBoardTiles(ctx context.Context, boardID string, placements []TilePlacement) error
	RemoveBoardTile(ctx context.Context, boardID, appID string) error
	ListAppBoards(ctx context.Context, appID string) ([]string, error)
	// ResizeLLMTiles resizes the app's tiles the user has not placed.
	ResizeLLMTiles(ctx context.Context, appID string, w, h int) error
	GetLoopApp(ctx context.Context, loopID string) (LoopApp, error)
	UpdateLoopApp(ctx context.Context, app LoopApp) error
	SetLoopAppError(ctx context.Context, appID, message string) error
	SetLoopAppLayout(ctx context.Context, appID, layout string) error
	// PurgeOrphanLoopApps takes the apps of deleted loops off their boards and
	// leaves the published ones in the library as ordinary apps.
	PurgeOrphanLoopApps(ctx context.Context) (int, error)
	ListLegacyWidgetHTML(ctx context.Context) ([]LegacyWidgetHTML, error)
	DeleteLegacyWidgetHTML(ctx context.Context, appID string) error
}
