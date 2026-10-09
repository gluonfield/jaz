// Package mcpsearch serves the search tools of connected MCP servers.
package mcpsearch

import (
	"context"
	"net/http"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mcp"
)

type Searcher interface {
	Search(ctx context.Context, query string) ([]mcp.SearchSection, error)
}

type Handler struct {
	searcher Searcher
}

type response struct {
	Sections []mcp.SearchSection `json:"sections"`
}

func NewHandler(searcher Searcher) Handler {
	return Handler{searcher: searcher}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sections, err := h.searcher.Search(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, response{Sections: sections})
}
