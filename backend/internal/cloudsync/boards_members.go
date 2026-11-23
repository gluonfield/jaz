package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Service) activeAuthedAccount() (storage.CloudAccount, error) {
	acc, _, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.CloudAccount{}, ErrNotConnected
	}
	return acc, nil
}

func (s *Service) ListMembers(ctx context.Context, query string, limit int) (json.RawMessage, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, err
	}
	path := "/v1/members"
	params := url.Values{}
	if q := strings.TrimSpace(query); q != "" {
		params.Set("q", q)
	}
	if limit > 0 {
		if limit > 200 {
			limit = 200
		}
		params.Set("limit", strconv.Itoa(limit))
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, path, acc.Token, nil)
	return raw, err
}

func (s *Service) CreateMember(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/members", acc.Token, body)
}

func (s *Service) UpdateMember(ctx context.Context, memberID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(memberID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodPatch, acc.BaseURL, "/v1/members/"+url.PathEscape(memberID), acc.Token, body)
}

func (s *Service) DeleteMember(ctx context.Context, memberID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(memberID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodDelete, acc.BaseURL, "/v1/members/"+url.PathEscape(memberID), acc.Token, nil)
}

func (s *Service) ListBoards(ctx context.Context) (json.RawMessage, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, err
	}
	raw, _, err := s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/boards", acc.Token, nil)
	return raw, err
}

func (s *Service) CreateBoard(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodPost, acc.BaseURL, "/v1/boards", acc.Token, body)
}

func (s *Service) GetBoard(ctx context.Context, boardID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(boardID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodGet, acc.BaseURL, "/v1/boards/"+url.PathEscape(boardID), acc.Token, nil)
}

func (s *Service) PatchBoard(ctx context.Context, boardID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(boardID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodPatch, acc.BaseURL, "/v1/boards/"+url.PathEscape(boardID), acc.Token, body)
}

func (s *Service) DeleteBoard(ctx context.Context, boardID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(boardID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, http.MethodDelete, acc.BaseURL, "/v1/boards/"+url.PathEscape(boardID), acc.Token, nil)
}

func (s *Service) DeleteBoardWidget(ctx context.Context, boardID, widgetID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(boardID); err != nil {
		return nil, 0, err
	}
	if err := validatePathSegment(widgetID); err != nil {
		return nil, 0, err
	}
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	path := "/v1/boards/" + url.PathEscape(boardID) + "/widgets/" + url.PathEscape(widgetID)
	return s.client.doJSON(ctx, http.MethodDelete, acc.BaseURL, path, acc.Token, nil)
}
