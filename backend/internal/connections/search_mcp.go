package connections

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const searchResultsMax = 20

type SearchInput struct {
	Query string `json:"query" jsonschema:"service name or what it does, such as slack or calendar"`
}

type SearchResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Accounts    []string `json:"accounts,omitempty"`
	ConnectLink string   `json:"connect_link,omitempty"`
}

type SearchOutput struct {
	Connections []SearchResult `json:"connections"`
}

func (s *Service) AddSearchTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "connections_search",
		Title:       "Search connections",
		Description: "Search the services Jaz can connect to, such as email, chat, calendars and MCP apps. Each match lists its connected accounts and, when it can be connected, a connect link. Search before saying a service is unavailable. To offer a connection, write its connect link alone on its own line; Jaz renders it as a card the user clicks to connect.",
	}, s.search)
}

func (s *Service) search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	plugins, err := s.ListPlugins(ctx, false)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	words := strings.Fields(strings.ToLower(input.Query))
	out := SearchOutput{Connections: []SearchResult{}}
	for _, plugin := range plugins {
		if len(out.Connections) == searchResultsMax {
			break
		}
		if plugin.Connection == nil || !matchesAll(plugin, words) {
			continue
		}
		result := SearchResult{Name: plugin.Name, Description: plugin.Description}
		for _, account := range plugin.Connection.Accounts {
			result.Accounts = append(result.Accounts, accountLabel(account))
		}
		if plugin.Available() && (len(result.Accounts) == 0 || plugin.MultiAccount) {
			result.ConnectLink = "jaz://connect/" + plugin.ID
		}
		out.Connections = append(out.Connections, result)
	}
	return nil, out, nil
}

func matchesAll(plugin integrations.Plugin, words []string) bool {
	text := strings.ToLower(strings.Join([]string{plugin.ID, plugin.Name, plugin.Provider.Name, plugin.Category, plugin.Description}, " "))
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
