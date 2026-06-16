package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/threads"
)

type threadSearchResponse struct {
	Results []ThreadSearchResult `json:"results"`
}

type ThreadSearchResult struct {
	ThreadID        string `json:"thread_id"`
	ThreadSlug      string `json:"thread_slug"`
	ThreadTitle     string `json:"thread_title"`
	ThreadStatus    string `json:"thread_status"`
	ThreadRuntime   string `json:"thread_runtime"`
	ParentID        string `json:"parent_id,omitempty"`
	Archived        bool   `json:"archived"`
	MessageSeq      int64  `json:"message_seq,omitempty"`
	Snippet         string `json:"snippet"`
	HitCount        int    `json:"hit_count"`
	UpdatedAt       string `json:"updated_at"`
	LastAttentionAt string `json:"last_attention_at"`
}

type ThreadSearchHandler struct {
	threads *threads.Service
}

func NewThreadSearchHandler(service *threads.Service) ThreadSearchHandler {
	return ThreadSearchHandler{threads: service}
}

func (h ThreadSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, errors.New("limit must be a non-negative integer"))
			return
		}
		limit = parsed
	}
	results, err := h.threads.Search(r.Context(), threads.SearchQuery{
		Query:           query,
		IncludeArchived: r.URL.Query().Get("include_archived") == "true",
		Limit:           limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, threadSearchResponse{Results: threadSearchResults(results)})
}

func threadSearchResults(results []threads.SearchResult) []ThreadSearchResult {
	out := make([]ThreadSearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, ThreadSearchResult{
			ThreadID:        result.ThreadID,
			ThreadSlug:      result.ThreadSlug,
			ThreadTitle:     result.ThreadTitle,
			ThreadStatus:    result.ThreadStatus,
			ThreadRuntime:   result.ThreadRuntime,
			ParentID:        result.ParentID,
			Archived:        result.Archived,
			MessageSeq:      result.MessageSeq,
			Snippet:         result.Snippet,
			HitCount:        result.HitCount,
			UpdatedAt:       result.UpdatedAt.Format(time.RFC3339Nano),
			LastAttentionAt: result.LastAttentionAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
