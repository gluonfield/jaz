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

func (s *Service) authedJSON(ctx context.Context, method, path string, body any) (json.RawMessage, int, error) {
	acc, err := s.activeAuthedAccount()
	if err != nil {
		return nil, 0, err
	}
	return s.client.doJSON(ctx, method, acc.BaseURL, path, acc.Token, body)
}

func (s *Service) authedSegmentJSON(ctx context.Context, method, prefix, segment, suffix string, body any) (json.RawMessage, int, error) {
	if err := validatePathSegment(segment); err != nil {
		return nil, 0, err
	}
	return s.authedJSON(ctx, method, prefix+url.PathEscape(segment)+suffix, body)
}

func (s *Service) ListMembers(ctx context.Context, query string, limit int) (json.RawMessage, error) {
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
	raw, _, err := s.authedJSON(ctx, http.MethodGet, path, nil)
	return raw, err
}

func (s *Service) CreateMember(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedJSON(ctx, http.MethodPost, "/v1/members", body)
}

func (s *Service) UpdateMember(ctx context.Context, memberID string, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodPatch, "/v1/members/", memberID, "", body)
}

func (s *Service) DeleteMember(ctx context.Context, memberID string) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodDelete, "/v1/members/", memberID, "", nil)
}

func (s *Service) ListBoards(ctx context.Context) (json.RawMessage, error) {
	raw, _, err := s.authedJSON(ctx, http.MethodGet, "/v1/boards", nil)
	return raw, err
}

func (s *Service) CreateBoard(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedJSON(ctx, http.MethodPost, "/v1/boards", body)
}

func (s *Service) GetBoard(ctx context.Context, boardID string) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodGet, "/v1/boards/", boardID, "", nil)
}

func (s *Service) PatchBoard(ctx context.Context, boardID string, body json.RawMessage) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodPatch, "/v1/boards/", boardID, "", body)
}

func (s *Service) DeleteBoard(ctx context.Context, boardID string) (json.RawMessage, int, error) {
	return s.authedSegmentJSON(ctx, http.MethodDelete, "/v1/boards/", boardID, "", nil)
}

func (s *Service) DeleteBoardWidget(ctx context.Context, boardID, widgetID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(boardID); err != nil {
		return nil, 0, err
	}
	if err := validatePathSegment(widgetID); err != nil {
		return nil, 0, err
	}
	return s.authedJSON(ctx, http.MethodDelete, "/v1/boards/"+url.PathEscape(boardID)+"/widgets/"+url.PathEscape(widgetID), nil)
}
