package widgets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/visualize"
)

const (
	WidgetFileName = "index.html"
)

type Service struct {
	Repo Repository
	Log  *log.Logger
	Now  func() time.Time
	mu   sync.Mutex
}

func NewService(repo Repository, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{Repo: repo, Log: logger.WithPrefix("widgets")}
}

type PublishInput struct {
	Title    string
	SizeHint string
	HTML     string
}

type RunPublishState struct {
	Widget    Widget
	Enabled   bool
	Published bool
}

// WidgetDir returns the loop's widget codebase directory, derived from the
// memory path so the widget lives next to memory.md.
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

// Publish validates and snapshots a new widget version for the loop. When
// input.HTML is empty the widget file is read from the loop's widget dir.
// A widget only exists by being assigned to a board, so publishing requires
// an assignment. The returned warnings are non-fatal lint findings for the
// publish channel to relay so the agent can fix them in the same run.
func (s *Service) Publish(loop loops.Loop, runID string, input PublishInput) (Widget, []string, error) {
	html := input.HTML
	if strings.TrimSpace(html) == "" {
		path := WidgetFilePath(loop)
		if path == "" {
			return Widget{}, nil, fmt.Errorf("loop %s has no widget directory; provide html inline", loop.ID)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Widget{}, nil, fmt.Errorf("read widget file %s: %w", path, err)
		}
		html = string(data)
	}
	if err := ValidateHTML(html); err != nil {
		return Widget{}, nil, err
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	widget, found, err := s.Repo.LoadWidgetByLoop(loop.ID)
	if err != nil {
		return Widget{}, nil, err
	}
	if !found {
		return Widget{}, nil, fmt.Errorf("loop %s is not assigned to any board; the user assigns loops to boards", loop.ID)
	}
	boards, err := s.Repo.ListBoardsForWidget(widget.ID)
	if err != nil {
		return Widget{}, nil, err
	}
	if len(boards) == 0 {
		return Widget{}, nil, fmt.Errorf("loop %s is not assigned to any board; the user assigns loops to boards", loop.ID)
	}
	if title := strings.TrimSpace(input.Title); title != "" {
		widget.Title = title
	}
	if widget.Title == "" {
		widget.Title = loop.Name
	}
	if hint := normalizeSizeHint(input.SizeHint); hint != "" {
		widget.SizeHint = hint
	}
	widget.CurrentVersion++
	widget.LastError = ""
	widget.LastLayout = ""
	widget.UpdatedAt = now
	if err := s.Repo.SaveWidgetWithVersion(widget, Version{
		WidgetID:        widget.ID,
		Version:         widget.CurrentVersion,
		HTML:            html,
		ProducedByRunID: runID,
		CreatedAt:       now,
	}); err != nil {
		return Widget{}, nil, err
	}
	if err := s.Repo.PruneOldWidgetVersions(widget.ID, widget.CurrentVersion, MaxOldVersions); err != nil {
		s.Log.Warn("pruning widget versions failed", "widget", widget.ID, "error", err)
	}
	s.applySizeHintLocked(widget, boards, now)
	return widget, LintHTML(html), nil
}

// StateForLoop reports the loop's widget and assigned boards.
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
	return PromptSection(loop, &widget)
}

func (s *Service) LoopArtifactSurface(loop loops.Loop, _ loops.Run) string {
	if s.WidgetEnabled(loop.ID) {
		return string(visualize.SurfaceWidget)
	}
	return ""
}

func (s *Service) WidgetEnabled(loopID string) bool {
	_, boards, found, err := s.StateForLoop(loopID)
	return err == nil && found && len(boards) > 0
}

func (s *Service) RunPublishState(loopID, runID string) (RunPublishState, error) {
	widget, found, err := s.Repo.LoadWidgetByLoop(loopID)
	if err != nil || !found {
		return RunPublishState{}, err
	}
	boards, err := s.Repo.ListBoardsForWidget(widget.ID)
	if err != nil {
		return RunPublishState{}, err
	}
	state := RunPublishState{Widget: widget, Enabled: len(boards) > 0}
	if !state.Enabled || widget.CurrentVersion <= 0 {
		return state, nil
	}
	version, err := s.Repo.LoadWidgetVersion(widget.ID, widget.CurrentVersion)
	if err != nil {
		return RunPublishState{}, err
	}
	state.Published = strings.TrimSpace(version.ProducedByRunID) == strings.TrimSpace(runID)
	return state, nil
}

func (s *Service) RequireRunPublished(loop loops.Loop, run loops.Run) error {
	state, err := s.RunPublishState(loop.ID, run.ID)
	if err != nil || !state.Enabled || state.Published {
		return err
	}
	err = fmt.Errorf("widget was not published by this run; call visualise_publish_widget after writing widget/index.html")
	s.reportPublishError(state.Widget.ID, err)
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
		return fmt.Errorf("widget html is empty")
	}
	if len(html) > MaxHTMLBytes {
		return fmt.Errorf("widget html is %d bytes; the limit is %d", len(html), MaxHTMLBytes)
	}
	if strings.ContainsRune(html, 0) {
		return fmt.Errorf("widget html contains a null byte")
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "<") {
		return fmt.Errorf("widget html does not look like markup")
	}
	for _, tag := range []string{"<!doctype", "<html", "<head", "<body"} {
		if strings.Contains(lower, tag) {
			return fmt.Errorf("widget html must be a fragment: remove the %s tag (Jaz wraps the fragment in its own document)", tag+">")
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
