package threads

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/storage"
)

type ListRequest struct {
	Limit    int  `json:"limit,omitempty" jsonschema:"Maximum threads to return, 1-50; defaults to 20."`
	Archived bool `json:"archived,omitempty" jsonschema:"List archived threads instead of current threads."`
}

type ThreadSummary struct {
	ThreadID  string    `json:"threadId"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
	Snippet   string    `json:"snippet,omitempty"`
}

type DiscoveryResponse struct {
	Threads []ThreadSummary `json:"threads"`
}

func (s *Service) ListMCP(ctx context.Context, _ *mcp.CallToolRequest, input ListRequest) (*mcp.CallToolResult, DiscoveryResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, DiscoveryResponse{}, err
	}
	sessions, err := s.context.ListSessions(storage.SessionFilter{
		IncludeChildren: true, IncludeSourced: true, Archived: input.Archived, Limit: clampSearchLimit(input.Limit),
	})
	out := DiscoveryResponse{Threads: make([]ThreadSummary, 0, len(sessions))}
	for _, session := range sessions {
		agent := ""
		if session.RuntimeRef != nil {
			agent = session.RuntimeRef.Agent
		}
		out.Threads = append(out.Threads, ThreadSummary{
			ThreadID: session.ID, Slug: session.Slug, Title: session.Title, Agent: agent,
			Status: session.Status, UpdatedAt: session.UpdatedAt,
		})
	}
	return nil, out, err
}

type SearchRequest struct {
	Query           string `json:"query" jsonschema:"Words to find in thread titles and messages."`
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum threads to return, 1-50; defaults to 20."`
	IncludeArchived bool   `json:"includeArchived,omitempty"`
}

func (s *Service) SearchMCP(ctx context.Context, _ *mcp.CallToolRequest, input SearchRequest) (*mcp.CallToolResult, DiscoveryResponse, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, DiscoveryResponse{}, fmt.Errorf("query is required")
	}
	results, err := s.Search(ctx, SearchQuery{Query: input.Query, IncludeArchived: input.IncludeArchived, IncludeSourced: true, Limit: input.Limit})
	out := DiscoveryResponse{Threads: make([]ThreadSummary, 0, len(results))}
	for _, result := range results {
		out.Threads = append(out.Threads, ThreadSummary{
			ThreadID: result.ThreadID, Slug: result.ThreadSlug, Title: result.ThreadTitle,
			Agent: result.ThreadAgent, Status: result.ThreadStatus, UpdatedAt: result.UpdatedAt, Snippet: result.Snippet,
		})
	}
	return nil, out, err
}
