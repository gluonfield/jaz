package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wins/jaz/backend/internal/storage"
)

var ErrWorkspaceChanged = errors.New("active Cloud workspace changed")

type AccountSelection struct {
	ID         string
	Generation uint64
}

func (s *Service) ActiveCloudAccount() (storage.CloudAccount, AccountSelection) {
	acc, gen, _ := s.activeAccountWithGen()
	if acc.Token == "" {
		return storage.CloudAccount{}, AccountSelection{}
	}
	return acc, AccountSelection{ID: acc.ID, Generation: gen}
}

func (s *Service) ListModelsForWorkspace(ctx context.Context, expected string) (json.RawMessage, error) {
	raw, _, err := s.selectedJSON(ctx, expected, http.MethodGet, "/v1/models", nil)
	if expected == "" && errors.Is(err, ErrNotConnected) {
		return json.RawMessage(`{"models":[]}`), nil
	}
	return raw, err
}

func (s *Service) ACPProfiles(ctx context.Context, expected, method, path string, body json.RawMessage) (json.RawMessage, int, error) {
	clean, err := cleanProxyPath(path)
	if err != nil {
		return nil, 0, err
	}
	return s.selectedJSON(ctx, expected, method, clean, body)
}

func (s *Service) selectedJSON(ctx context.Context, expected, method, path string, body json.RawMessage) (json.RawMessage, int, error) {
	acc, selection := s.ActiveCloudAccount()
	if expected != "" && expected != selection.ID {
		return nil, 0, ErrWorkspaceChanged
	}
	if selection.ID == "" {
		return nil, 0, ErrNotConnected
	}
	client := s.clientFor(acc.ID)
	refresh := client.refresh
	client.refresh = func(ctx context.Context, id, token string) (string, error) {
		if !s.isGeneration(selection.Generation) {
			return "", ErrWorkspaceChanged
		}
		next, err := refresh(ctx, id, token)
		if !s.isGeneration(selection.Generation) {
			return "", ErrWorkspaceChanged
		}
		return next, err
	}
	raw, status, err := client.doJSON(ctx, method, acc.BaseURL, path, acc.Token, body)
	if !s.isGeneration(selection.Generation) {
		return nil, 0, ErrWorkspaceChanged
	}
	return raw, status, err
}
