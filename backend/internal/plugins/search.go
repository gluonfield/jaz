package plugins

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/storage"
)

type SearchInput struct {
	Query string `json:"query" jsonschema:"plugin or service name or what it does, such as gmail, slack, calendar or tenders"`
}

type SearchResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Installed   bool     `json:"installed"`
	Enabled     bool     `json:"enabled"`
	Accounts    []string `json:"accounts,omitempty"`
	ConnectLink string   `json:"connect_link,omitempty"`
}

type SearchOutput struct {
	Plugins []SearchResult `json:"plugins"`
}

func AddSearchTools(server *mcp.Server, store *Store, connService *connections.Service) {
	var state storage.PluginStore
	if store != nil {
		state = store.stateStore
	}
	svc := &Service{store: store, state: state, connections: connService}
	svc.AddSearchTo(server)
}

func (s *Service) AddSearchTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        SearchMCPToolName,
		Title:       "Search plugins",
		Description: "Search plugins and services Jaz can connect to, such as email, chat, calendars, skills, and MCP apps. Each match lists its connected accounts and, when it can be connected, a connect link. Search before saying a service is unavailable. To offer a connection, write its connect link alone on its own line; Jaz renders it as a card the user clicks to connect.",
	}, s.searchMCP)
}

func (s *Service) searchMCP(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	catalog, err := s.List(ctx, false)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	words := strings.Fields(strings.ToLower(input.Query))
	out := SearchOutput{Plugins: []SearchResult{}}
	for _, plugin := range catalog.Plugins {
		if len(out.Plugins) == searchResultsMax {
			break
		}
		if !matchesSearchWords(plugin, words) {
			continue
		}
		result := SearchResult{
			ID:          plugin.ID,
			Name:        plugin.DisplayName,
			Description: plugin.Description,
			Installed:   plugin.Installed,
			Enabled:     plugin.Enabled,
		}
		if plugin.Auth != nil {
			for _, account := range plugin.Auth.Accounts {
				result.Accounts = append(result.Accounts, connections.AccountLabel(account))
			}
			if plugin.Auth.Status == "available" && (!plugin.Auth.Connected || plugin.Auth.MultiAccount) {
				result.ConnectLink = "jaz://plugin/" + plugin.ID + "/connect"
			}
		}
		out.Plugins = append(out.Plugins, result)
	}
	return nil, out, nil
}

func matchesSearchWords(plugin Plugin, words []string) bool {
	if len(words) == 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		plugin.ID,
		plugin.DisplayName,
		plugin.Category,
		plugin.Description,
		strings.Join(plugin.Keywords, " "),
	}, " "))
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
