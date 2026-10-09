package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// searchTool is the tool OpenAI's MCP search convention names: it takes a
// query and returns {results: [{id, title, url, text}]} as structured content.
const searchTool = "search"

const searchTimeout = 5 * time.Second

type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text,omitempty"`
}

// SearchSection is what one connected server found.
type SearchSection struct {
	ServerID string         `json:"server_id"`
	Name     string         `json:"name"`
	Icon     string         `json:"icon,omitempty"`
	Results  []SearchResult `json:"results"`
}

// isSearchTool reports whether Jaz may call a tool as the server's search. It
// runs as a person types, so only a tool declared read-only qualifies.
func isSearchTool(tool *mcpsdk.Tool) bool {
	return tool.Name == searchTool && tool.Annotations != nil && tool.Annotations.ReadOnlyHint
}

// Search asks every connected server with a search tool at once. A server
// that fails, misses the deadline or finds nothing is left out.
func (m *Manager) Search(ctx context.Context, query string) ([]SearchSection, error) {
	servers, err := m.store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	sections := make([]SearchSection, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		session := m.session(server.ID)
		if session == nil || !session.searchable {
			continue
		}
		wg.Go(func() {
			results, err := session.search(ctx, query)
			if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
				m.log.Warn("MCP search failed", "server", server.ID, "error", err)
			}
			sections[i] = SearchSection{ServerID: server.ID, Name: server.Name, Icon: session.apps.icon, Results: results}
		})
	}
	wg.Wait()
	return slices.DeleteFunc(sections, func(section SearchSection) bool { return len(section.Results) == 0 }), nil
}

// search keeps the results a person can open: titled, with a web link.
func (s *serverSession) search(ctx context.Context, query string) ([]SearchResult, error) {
	result, err := s.callTool(ctx, &mcpsdk.CallToolParams{Name: searchTool, Arguments: map[string]string{"query": query}})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, errors.New("search tool returned an error")
	}
	var found struct {
		Results []SearchResult `json:"results"`
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &found); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(found.Results, func(hit SearchResult) bool {
		link, err := url.Parse(hit.URL)
		return hit.Title == "" || err != nil || link.Host == "" || link.Scheme != "http" && link.Scheme != "https"
	}), nil
}
