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

func (s *ViewService) shown(ctx context.Context, session, reference, file string) (string, []sessionevents.PluginView, error) {
	plugin, view, _ := strings.Cut(reference, "/")
	if !name.MatchString(plugin) || !name.MatchString(view) || !validViewFile(file) {
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

func (s *ViewService) State(ctx context.Context, session, reference, file string) (json.RawMessage, error) {
	state, _, err := s.StateWithRevision(ctx, session, reference, file)
	return state, err
}

func (s *ViewService) StateWithRevision(ctx context.Context, session, reference, file string) (json.RawMessage, int64, error) {
	thread, _, err := s.shown(ctx, session, reference, file)
	if err != nil {
		return nil, 0, err
	}
	plugin, view, _ := strings.Cut(reference, "/")
	if s.plugins != nil && s.plugins.stateStore != nil {
		record, ok, err := s.plugins.stateStore.GetThreadPluginState(ctx, thread, plugin, view, file)
		if err != nil {
			return nil, 0, err
		}
		if ok && len(record.UIStateJSON) > 0 {
			return record.UIStateJSON, record.Revision, nil
		}
	}
	state, err := os.ReadFile(filepath.Join(s.states, thread, reference, filepath.FromSlash(file)))
	if errors.Is(err, fs.ErrNotExist) {
		return json.RawMessage("null"), 0, nil
	}
	return state, 0, err
}

func (s *ViewService) SaveState(ctx context.Context, session, reference, file string, state json.RawMessage) error {
	_, err := s.SaveStateWithRevision(ctx, session, reference, file, state, nil)
	return err
}

func (s *ViewService) SaveStateWithRevision(ctx context.Context, session, reference, file string, state json.RawMessage, expectedRevision *int64) (int64, error) {
	if !json.Valid(state) {
		return 0, fmt.Errorf("%w: view state is not JSON", ErrInvalid)
	}
	thread, _, err := s.shown(ctx, session, reference, file)
	if err != nil {
		return 0, err
	}
	plugin, view, _ := strings.Cut(reference, "/")
	if s.plugins != nil && s.plugins.stateStore != nil {
		if err := s.plugins.stateStore.SaveThreadPluginUIState(ctx, thread, plugin, view, file, state, expectedRevision); err != nil {
			return 0, err
		}
		record, ok, err := s.plugins.stateStore.GetThreadPluginState(ctx, thread, plugin, view, file)
		if err != nil {
			return 0, err
		}
		if ok {
			return record.Revision, nil
		}
		return 1, nil
	}
	target := filepath.Join(s.states, thread, reference, filepath.FromSlash(file))
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+"-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(temp.Name())
	_, err = temp.Write(state)
	if err = errors.Join(err, temp.Close()); err != nil {
		return 0, err
	}
	return 0, os.Rename(temp.Name(), target)
}
