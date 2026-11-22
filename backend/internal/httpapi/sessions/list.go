package sessions

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
)

type sessionListStore interface {
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
	LastRootSession() (storage.Session, error)
}

type CloudSessionLister interface {
	ListSessions(context.Context, bool) ([]storage.Session, error)
}

type ListHandler struct {
	store sessionListStore
	cloud CloudSessionLister
}

type listResponse struct {
	Sessions   []sessionview.Response `json:"sessions"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

func NewListHandler(store sessionListStore) *ListHandler {
	return &ListHandler{store: store}
}

func (h *ListHandler) WithCloud(cloud CloudSessionLister) *ListHandler {
	h.cloud = cloud
	return h
}

func (h *ListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("last") == "true" {
		session, err := h.store.LastRootSession()
		if err != nil {
			httpapi.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, sessionview.Public(session))
		return
	}
	limit := 0
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and 200"))
			return
		}
	}
	filter := storage.SessionFilter{
		Query:           strings.TrimSpace(query.Get("q")),
		ParentID:        query.Get("parent_id"),
		ParentOnly:      query.Has("parent_id"),
		RootOnly:        query.Get("root") == "true",
		Runtime:         query.Get("runtime"),
		IncludeChildren: query.Get("include_children") == "true",
		SourceType:      query.Get("source_type"),
		SourceID:        query.Get("source_id"),
		IncludeSourced:  query.Get("include_sourced") == "true",
		Archived:        query.Get("archived") == "true",
		Limit:           limit,
	}
	if raw := strings.TrimSpace(query.Get("updated_since")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("updated_since must be RFC3339: %w", err))
			return
		}
		filter.UpdatedSince = parsed
	}
	if raw := query.Get("cursor"); raw != "" {
		timestamp, id, ok := strings.Cut(raw, ":")
		ms, err := strconv.ParseInt(timestamp, 10, 64)
		if !ok || err != nil || id == "" || limit == 0 {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("cursor must be a timestamp and thread ID, with a limit"))
			return
		}
		filter.After = &storage.SessionPosition{AttentionAt: time.UnixMilli(ms), ID: id}
	}
	if limit > 0 {
		filter.Limit++
	}

	sessions, err := h.store.ListSessions(filter)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if h.cloud != nil && !filter.ParentOnly {
		if cloudSessions, cloudErr := h.cloud.ListSessions(r.Context(), filter.Archived); cloudErr == nil && len(cloudSessions) > 0 {
			sessions = mergeSessionLists(sessions, filterCloudSessions(cloudSessions, filter))
		}
	}
	var nextCursor string
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
		last := sessions[len(sessions)-1]
		nextCursor = fmt.Sprintf("%d:%s", storage.SessionAttentionAt(last).UnixMilli(), last.ID)
	}
	httpapi.WriteJSON(w, http.StatusOK, listResponse{Sessions: sessionview.Responses(sessions), NextCursor: nextCursor})
}

func filterCloudSessions(items []storage.Session, filter storage.SessionFilter) []storage.Session {
	q := strings.ToLower(filter.Query)
	out := make([]storage.Session, 0, len(items))
	for _, s := range items {
		if filter.Runtime != "" && s.Runtime != filter.Runtime {
			continue
		}
		if filter.SourceType != "" && s.SourceType != filter.SourceType {
			continue
		}
		if filter.SourceID != "" && s.SourceID != filter.SourceID {
			continue
		}
		if !filter.IncludeSourced && filter.SourceType == "" && s.SourceType != "" {
			continue
		}
		if !filter.UpdatedSince.IsZero() && !s.UpdatedAt.After(filter.UpdatedSince) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(s.Title), q) && !strings.Contains(strings.ToLower(s.ID), q) {
			continue
		}
		if filter.After != nil {
			at := storage.SessionAttentionAt(s)
			if at.After(filter.After.AttentionAt) || (at.Equal(filter.After.AttentionAt) && s.ID <= filter.After.ID) {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func mergeSessionLists(local, cloud []storage.Session) []storage.Session {
	if len(cloud) == 0 {
		return local
	}
	seen := make(map[string]struct{}, len(local)+len(cloud))
	merged := make([]storage.Session, 0, len(local)+len(cloud))
	for _, s := range local {
		seen[s.ID] = struct{}{}
		merged = append(merged, s)
	}
	for _, s := range cloud {
		if _, ok := seen[s.ID]; !ok {
			seen[s.ID] = struct{}{}
			merged = append(merged, s)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		ti := storage.SessionAttentionAt(merged[i])
		tj := storage.SessionAttentionAt(merged[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return merged[i].ID < merged[j].ID
	})
	return merged
}
