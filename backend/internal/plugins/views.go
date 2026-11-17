package plugins

import (
	"context"
	"fmt"
	"strings"

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
}

type ViewRequest struct {
	Reference string
	File      string
	Name      string
	Title     string
}

func NewViewService(sessions storage.SessionOverviewReader, presenter ViewPresenter, plugins *Store) *ViewService {
	return &ViewService{plugins: plugins, sessions: sessions, presenter: presenter}
}

func (s *ViewService) Open(ctx context.Context, session string, request ViewRequest) (sessionevents.PluginView, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Title = strings.TrimSpace(request.Title)
	if !name.MatchString(request.Name) || request.Title == "" {
		return sessionevents.PluginView{}, fmt.Errorf("%w: a plain view name and title are required", ErrInvalid)
	}
	view, err := s.sessions.LoadSessionOverview(ctx, session)
	if err != nil {
		return sessionevents.PluginView{}, err
	}
	for _, caller := range sessionevents.LatestPluginViews(view.PresentationEvents) {
		if caller.Reference != request.Reference || caller.File != request.File {
			continue
		}
		plugin, _, ok := strings.Cut(caller.Reference, "/")
		if !ok || !filename.MatchString(caller.File) {
			return sessionevents.PluginView{}, fmt.Errorf("%w: caller has no owned file-following view", ErrInvalid)
		}
		reference := plugin + "/" + request.Name
		html, err := s.plugins.View(reference)
		if err != nil {
			return sessionevents.PluginView{}, err
		}
		target := sessionevents.PluginView{Reference: reference, Title: request.Title, WidgetCode: html, File: caller.File}
		_, err = s.presenter.Present(view.SessionID, &sessionevents.PresentationEvent{Kind: sessionevents.PresentationPluginView, View: &target})
		return target, err
	}
	return sessionevents.PluginView{}, fmt.Errorf("%w: caller view is not shown in this thread", ErrNotFound)
}
