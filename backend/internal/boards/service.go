package boards

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

const (
	GridCols     = 6
	RowHeight    = 120
	MaxHTMLBytes = 1 << 20
	appFileName  = "index.html"
)

type Store interface {
	storage.BoardStore
	GetApp(ctx context.Context, appID string) (storage.App, error)
	ListApps(ctx context.Context, workspaceID string, filter storage.AppListFilter) ([]storage.App, error)
}

// LoopApps owns the app a loop keeps on its boards.
type LoopApps interface {
	EnsureLoopApp(ctx context.Context, loopID, title string) (storage.App, error)
	PublishLoopApp(ctx context.Context, appID, title, html, sourceThreadID string) (storage.App, error)
}

// Service lays apps out as tiles on boards. A board is an app of kind board;
// a loop is on a board when its app is a tile there.
type Service struct {
	store     Store
	apps      LoopApps
	log       *log.Logger
	workspace func() string
	mu        sync.Mutex
}

func NewService(store Store, workspace func() string, logger *log.Logger) *Service {
	return &Service{store: store, workspace: workspace, log: logger.WithPrefix("boards")}
}

// WithApps is set after construction: the apps service is built from the
// agent runtime, which is built from this service.
func (s *Service) WithApps(apps LoopApps) *Service {
	s.apps = apps
	return s
}

// View is a board's layout: its tiles and the grid they sit on.
type View struct {
	FontScale float64             `json:"font_scale"`
	GridCols  int                 `json:"grid_cols"`
	RowHeight int                 `json:"row_height"`
	Tiles     []storage.BoardTile `json:"tiles"`
}

func (s *Service) ListBoards(ctx context.Context) ([]storage.App, error) {
	return s.store.ListApps(ctx, s.workspace(), storage.AppListFilter{Kind: storage.AppKindBoard})
}

func (s *Service) board(ctx context.Context, id string) (storage.App, error) {
	app, err := s.store.GetApp(ctx, id)
	if err != nil {
		return storage.App{}, err
	}
	if app.Kind != storage.AppKindBoard || app.WorkspaceID != s.workspace() {
		return storage.App{}, storage.ErrAppNotFound
	}
	return app, nil
}

func (s *Service) View(ctx context.Context, boardID string) (View, error) {
	if _, err := s.board(ctx, boardID); err != nil {
		return View{}, err
	}
	scale, err := s.store.BoardFontScale(ctx, boardID)
	if err != nil {
		return View{}, err
	}
	tiles, err := s.store.ListBoardTiles(ctx, boardID)
	if err != nil {
		return View{}, err
	}
	return View{FontScale: scale, GridCols: GridCols, RowHeight: RowHeight, Tiles: tiles}, nil
}

type PublishInput struct {
	Title    string
	SizeHint string
	HTML     string
}

// Published is the app a loop run saved for its board tiles. Warnings are
// non-fatal lint findings for the publish channel to relay so the agent can
// fix them in the same run.
type Published struct {
	App      storage.App
	SizeHint string
	Warnings []string
}

// AppFilePath is where a loop run writes its app, next to the loop's memory.
func AppFilePath(loop loops.Loop) string {
	memory := strings.TrimSpace(loop.MemoryPath)
	if memory == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(memory), "widget", appFileName)
}

// Publish saves the HTML as the next version of the loop's app, which its
// board tiles show. When input.HTML is empty the loop's app file is read. A
// loop has an app only while it is on a board.
func (s *Service) Publish(ctx context.Context, loop loops.Loop, runID, sessionID string, input PublishInput) (Published, error) {
	html := input.HTML
	if strings.TrimSpace(html) == "" {
		path := AppFilePath(loop)
		if path == "" {
			return Published{}, fmt.Errorf("loop %s has no app directory; provide html inline", loop.ID)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Published{}, fmt.Errorf("read app file %s: %w", path, err)
		}
		html = string(data)
	}
	if err := ValidateHTML(html); err != nil {
		return Published{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	loopApp, boardIDs, err := s.StateForLoop(ctx, loop.ID)
	if err != nil {
		return Published{}, err
	}
	if len(boardIDs) == 0 {
		return Published{}, fmt.Errorf("loop %s is not assigned to any board; the user assigns loops to boards", loop.ID)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		current, err := s.store.GetApp(ctx, loopApp.AppID)
		if err != nil {
			return Published{}, err
		}
		title = current.Title
	}
	app, err := s.apps.PublishLoopApp(ctx, loopApp.AppID, title, html, sessionID)
	if err != nil {
		return Published{}, err
	}
	if w, h, ok := parseSize(input.SizeHint); ok {
		loopApp.SizeHint = fmt.Sprintf("%dx%d", w, h)
	}
	loopApp.PublishedRunID = runID
	loopApp.LastError = ""
	loopApp.LastLayout = ""
	if err := s.store.UpdateLoopApp(ctx, loopApp); err != nil {
		return Published{}, err
	}
	if w, h, ok := parseSize(loopApp.SizeHint); ok {
		if err := s.store.ResizeLLMTiles(ctx, app.ID, w, h); err != nil {
			s.log.Warn("resizing board tiles failed", "app", app.ID, "error", err)
		}
	}
	return Published{App: app, SizeHint: loopApp.SizeHint, Warnings: LintHTML(html)}, nil
}

// AdoptLegacyHTML turns HTML that loops published before tiles became apps
// into versions of their apps. Each conversion deletes its source, so an app
// converts once.
func (s *Service) AdoptLegacyHTML(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	legacy, err := s.store.ListLegacyWidgetHTML(ctx)
	if err != nil {
		return err
	}
	for _, tile := range legacy {
		if _, err := s.apps.PublishLoopApp(ctx, tile.AppID, tile.Title, tile.HTML, ""); err != nil {
			s.log.Warn("converting board tile to an app failed", "app", tile.AppID, "error", err)
			continue
		}
		if err := s.store.DeleteLegacyWidgetHTML(ctx, tile.AppID); err != nil {
			return err
		}
	}
	return nil
}

// StateForLoop reports the loop's app and the boards it is a tile on. A loop
// that was never on a board has neither.
func (s *Service) StateForLoop(ctx context.Context, loopID string) (storage.LoopApp, []string, error) {
	loopApp, err := s.store.GetLoopApp(ctx, loopID)
	if errors.Is(err, storage.ErrAppNotFound) {
		return storage.LoopApp{}, nil, nil
	}
	if err != nil {
		return storage.LoopApp{}, nil, err
	}
	boardIDs, err := s.store.ListAppBoards(ctx, loopApp.AppID)
	return loopApp, boardIDs, err
}

func (s *Service) LoopPromptExtra(loop loops.Loop, _ loops.Run) string {
	ctx := context.Background()
	loopApp, boardIDs, err := s.StateForLoop(ctx, loop.ID)
	if err != nil || len(boardIDs) == 0 {
		return ""
	}
	app, err := s.store.GetApp(ctx, loopApp.AppID)
	if err != nil {
		return ""
	}
	return PromptSection(loop, loopApp, app)
}

func (s *Service) LoopArtifactSurface(loop loops.Loop, _ loops.Run) string {
	if _, boardIDs, err := s.StateForLoop(context.Background(), loop.ID); err == nil && len(boardIDs) > 0 {
		return string(visualize.SurfaceWidget)
	}
	return ""
}

func (s *Service) RequireRunPublished(loop loops.Loop, run loops.Run) error {
	ctx := context.Background()
	loopApp, boardIDs, err := s.StateForLoop(ctx, loop.ID)
	if err != nil || len(boardIDs) == 0 || loopApp.PublishedRunID == run.ID {
		return err
	}
	err = fmt.Errorf("this run did not publish the board app; call %s with the finished HTML", PublishMCPToolName)
	_ = s.store.SetLoopAppError(ctx, loopApp.AppID, err.Error())
	return err
}

// Feedback is what a board measured while showing an app's current version.
// A loop's next run reads it; other apps have no run to tell.
type Feedback struct {
	Error  string
	Layout string
}

func (s *Service) ReportTile(ctx context.Context, appID string, feedback Feedback) error {
	if message := truncate(strings.TrimSpace(feedback.Error), 2000); message != "" {
		if err := s.store.SetLoopAppError(ctx, appID, message); err != nil {
			return err
		}
	}
	if layout := truncate(strings.TrimSpace(feedback.Layout), 1000); layout != "" {
		return s.store.SetLoopAppLayout(ctx, appID, layout)
	}
	return nil
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func ValidateHTML(html string) error {
	trimmed := strings.TrimSpace(html)
	if trimmed == "" {
		return fmt.Errorf("app html is empty")
	}
	if len(html) > MaxHTMLBytes {
		return fmt.Errorf("app html is %d bytes; the limit is %d", len(html), MaxHTMLBytes)
	}
	if strings.ContainsRune(html, 0) {
		return fmt.Errorf("app html contains a null byte")
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "<") {
		return fmt.Errorf("app html does not look like markup")
	}
	for _, tag := range []string{"<!doctype", "<html", "<head", "<body"} {
		if strings.Contains(lower, tag) {
			return fmt.Errorf("a board app must be an HTML fragment: remove the %s tag (Jaz wraps the fragment in its own document)", tag+">")
		}
	}
	return nil
}
