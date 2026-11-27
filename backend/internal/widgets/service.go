package widgets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
)

var ErrNotFound = errors.New("not found")

const (
	WidgetFileName = "index.html"
)

// LoopApps publishes the app a routine keeps on its boards.
type LoopApps interface {
	PublishLoopApp(ctx context.Context, loopID, title, html, sourceThreadID string) (storage.App, error)
	LoopApp(ctx context.Context, loopID string) (storage.App, error)
}

type Service struct {
	Repo      Repository
	Log       *log.Logger
	apps      LoopApps
	Now       func() time.Time
	workspace func() string
	mu        sync.Mutex
}

func NewService(repo Repository, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{Repo: repo, Log: logger.WithPrefix("widgets")}
}

// WithApps is set after construction: the apps service is built from the
// agent runtime, which is built from this service.
func (s *Service) WithApps(apps LoopApps) *Service {
	s.apps = apps
	return s
}

func (s *Service) WithWorkspace(fn func() string) *Service {
	s.workspace = fn
	return s
}

func (s *Service) ActiveWorkspaceID() string {
	if s.workspace != nil {
		if ws := s.workspace(); ws != "" {
			return ws
		}
	}
	return storage.LocalWorkspaceID
}

func normalizeWorkspaceID(workspaceID string) string {
	if workspaceID == "" {
		return storage.LocalWorkspaceID
	}
	return workspaceID
}

func (s *Service) ListBoards() ([]Board, error) {
	ws := s.ActiveWorkspaceID()
	if wsRepo, ok := s.Repo.(interface {
		ListBoardsForWorkspace(string) ([]Board, error)
	}); ok {
		return wsRepo.ListBoardsForWorkspace(ws)
	}
	boards, err := s.Repo.ListBoards()
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(boards))
	for _, board := range boards {
		if normalizeWorkspaceID(board.WorkspaceID) == ws {
			out = append(out, board)
		}
	}
	return out, nil
}

func (s *Service) LoadBoard(id string) (Board, error) {
	board, err := s.Repo.LoadBoard(id)
	if err != nil {
		return Board{}, err
	}
	if normalizeWorkspaceID(board.WorkspaceID) != s.ActiveWorkspaceID() {
		return Board{}, ErrNotFound
	}
	return board, nil
}

func (s *Service) LoadWidget(id string) (Widget, error) {
	widget, err := s.Repo.LoadWidget(id)
	if err != nil {
		return Widget{}, err
	}
	if normalizeWorkspaceID(widget.WorkspaceID) != s.ActiveWorkspaceID() {
		return Widget{}, ErrNotFound
	}
	return widget, nil
}

func (s *Service) ListBoardItems(boardID string) ([]BoardItem, error) {
	if _, err := s.LoadBoard(boardID); err != nil {
		return nil, err
	}
	return s.Repo.ListBoardItems(boardID)
}

type PublishInput struct {
	Title    string
	SizeHint string
	HTML     string
}

// Published is the app a routine run saved for its board tile. Warnings are
// non-fatal lint findings for the publish channel to relay so the agent can
// fix them in the same run.
type Published struct {
	App      storage.App
	SizeHint string
	Warnings []string
}

// WidgetDir returns the directory holding the loop's app source, derived from
// the memory path so it lives next to memory.md.
func WidgetDir(loop loops.Loop) string {
	memory := strings.TrimSpace(loop.MemoryPath)
	if memory == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(memory), "widget")
}

func WidgetFilePath(loop loops.Loop) string {
	dir := WidgetDir(loop)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, WidgetFileName)
}

// Publish saves the HTML as the next version of the loop's app, which its
// board tiles show. When input.HTML is empty the file in the loop's widget dir
// is read. A loop has a tile only while it is assigned to a board, so
// publishing requires an assignment.
func (s *Service) Publish(ctx context.Context, loop loops.Loop, runID, sessionID string, input PublishInput) (Published, error) {
	html := input.HTML
	if strings.TrimSpace(html) == "" {
		path := WidgetFilePath(loop)
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

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	widget, boards, found, err := s.StateForLoop(loop.ID)
	if err != nil {
		return Published{}, err
	}
	if !found || len(boards) == 0 {
		return Published{}, fmt.Errorf("loop %s is not assigned to any board; the user assigns loops to boards", loop.ID)
	}
	if title := strings.TrimSpace(input.Title); title != "" {
		widget.Title = title
	}
	if widget.Title == "" {
		widget.Title = loop.Name
	}
	app, err := s.apps.PublishLoopApp(ctx, loop.ID, widget.Title, html, sessionID)
	if err != nil {
		return Published{}, err
	}
	if hint := normalizeSizeHint(input.SizeHint); hint != "" {
		widget.SizeHint = hint
	}
	legacy := widget.CurrentVersion > 0
	widget.CurrentVersion = 0
	widget.PublishedRunID = runID
	widget.LastError = ""
	widget.LastLayout = ""
	widget.UpdatedAt = now
	if err := s.Repo.SaveWidget(widget); err != nil {
		return Published{}, err
	}
	if legacy {
		if err := s.Repo.DeleteWidgetVersions(widget.ID); err != nil {
			s.Log.Warn("deleting replaced widget versions failed", "widget", widget.ID, "error", err)
		}
	}
	s.applySizeHintLocked(widget, boards, now)
	return Published{App: app, SizeHint: widget.SizeHint, Warnings: LintHTML(html)}, nil
}

// StateForLoop reports the loop's tile and assigned boards.
func (s *Service) StateForLoop(loopID string) (Widget, []string, bool, error) {
	widget, found, err := s.Repo.LoadWidgetByLoop(loopID)
	if err != nil || !found {
		return Widget{}, nil, false, err
	}
	boards, err := s.Repo.ListBoardsForWidget(widget.ID)
	if err != nil {
		return Widget{}, nil, false, err
	}
	return widget, boards, true, nil
}

func (s *Service) LoopPromptExtra(loop loops.Loop, _ loops.Run) string {
	widget, boards, found, err := s.StateForLoop(loop.ID)
	if err != nil || !found || len(boards) == 0 {
		return ""
	}
	version := widget.CurrentVersion
	if app, err := s.apps.LoopApp(context.Background(), loop.ID); err == nil {
		version = app.CurrentVersion
	}
	return PromptSection(loop, widget, version)
}

func (s *Service) LoopArtifactSurface(loop loops.Loop, _ loops.Run) string {
	if _, boards, found, err := s.StateForLoop(loop.ID); err == nil && found && len(boards) > 0 {
		return string(visualize.SurfaceWidget)
	}
	return ""
}

func (s *Service) RequireRunPublished(loop loops.Loop, run loops.Run) error {
	widget, boards, found, err := s.StateForLoop(loop.ID)
	if err != nil || !found || len(boards) == 0 || widget.PublishedRunID == run.ID {
		return err
	}
	err = fmt.Errorf("this run did not publish the board app; call %s with the finished HTML", PublishMCPToolName)
	s.reportPublishError(widget.ID, err)
	return err
}

func (s *Service) reportPublishError(widgetID string, err error) {
	if widgetID != "" && err != nil {
		_ = s.ReportError(widgetID, err.Error())
	}
}

func (s *Service) ReportError(widgetID, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	widget, err := s.Repo.LoadWidget(widgetID)
	if err != nil {
		return err
	}
	widget.LastError = message
	return s.Repo.SaveWidget(widget)
}

// ReportLayout stores the board's layout telemetry (JSON from the bridge) for
// the widget's current version; the next run's prompt surfaces problems.
func (s *Service) ReportLayout(widgetID, payload string) error {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil
	}
	if len(payload) > 1000 {
		payload = payload[:1000]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	widget, err := s.Repo.LoadWidget(widgetID)
	if err != nil {
		return err
	}
	widget.LastLayout = payload
	return s.Repo.SaveWidget(widget)
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

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
