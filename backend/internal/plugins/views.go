package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/runtimefiles"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type ViewPresenter interface {
	Present(string, *sessionevents.PresentationEvent) ([]string, error)
}

type ViewService struct {
	plugins   *Store
	sessions  storage.SessionOverviewReader
	presenter ViewPresenter
	states    string
}

type ViewRequest struct {
	Reference string
	File      string
	Name      string
	Title     string
}

func NewViewService(sessions storage.SessionOverviewReader, presenter ViewPresenter, plugins *Store, layout runtimefiles.Layout) *ViewService {
	return &ViewService{plugins: plugins, sessions: sessions, presenter: presenter, states: layout.ViewStates}
}

func (s *ViewService) Open(ctx context.Context, session string, request ViewRequest) (sessionevents.PluginView, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Title = strings.TrimSpace(request.Title)
	if !name.MatchString(request.Name) || request.Title == "" {
		return sessionevents.PluginView{}, fmt.Errorf("%w: a plain view name and title are required", ErrInvalid)
	}
	thread, latest, err := s.shown(ctx, session, request.Reference, request.File)
	if err != nil {
		return sessionevents.PluginView{}, err
	}
	plugin, _, _ := strings.Cut(request.Reference, "/")
	reference := plugin + "/" + request.Name
	html, err := s.plugins.View(reference)
	if err != nil {
		return sessionevents.PluginView{}, err
	}
	for _, shown := range latest {
		if shown.Reference == reference && shown.File == request.File && shown.WidgetCode == html {
			return shown, nil
		}
	}
	target := sessionevents.PluginView{Reference: reference, Title: request.Title, WidgetCode: html, File: request.File}
	_, err = s.presenter.Present(thread, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &target})
	return target, err
}

// shown returns the thread's id and latest views when the file-following view
// named is one of them.
func (s *ViewService) shown(ctx context.Context, session, reference, file string) (string, []sessionevents.PluginView, error) {
	plugin, view, _ := strings.Cut(reference, "/")
	if !name.MatchString(plugin) || !name.MatchString(view) || !filename.MatchString(file) {
		return "", nil, fmt.Errorf("%w: not an owned file-following view", ErrInvalid)
	}
	thread, err := s.sessions.LoadSessionOverview(ctx, session)
	if err != nil {
		return "", nil, err
	}
	latest := sessionevents.LatestPluginViews(thread.PresentationEvents)
	if !slices.ContainsFunc(latest, func(shown sessionevents.PluginView) bool {
		return shown.Reference == reference && shown.File == file
	}) {
		return "", nil, fmt.Errorf("%w: view is not shown in this thread", ErrNotFound)
	}
	return thread.SessionID, latest, nil
}

// State returns what the view last saved for this thread: its own JSON, kept
// in one file that each save replaces. A view that saved nothing gets null.
func (s *ViewService) State(ctx context.Context, session, reference, file string) (json.RawMessage, error) {
	thread, _, err := s.shown(ctx, session, reference, file)
	if err != nil {
		return nil, err
	}
	state, err := os.ReadFile(filepath.Join(s.states, thread, reference, file))
	if errors.Is(err, fs.ErrNotExist) {
		return json.RawMessage("null"), nil
	}
	return state, err
}

func (s *ViewService) SaveState(ctx context.Context, session, reference, file string, state json.RawMessage) error {
	if !json.Valid(state) {
		return fmt.Errorf("%w: view state is not JSON", ErrInvalid)
	}
	thread, _, err := s.shown(ctx, session, reference, file)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.states, thread, reference)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+file+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, err = temp.Write(state)
	if err = errors.Join(err, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, file))
}
