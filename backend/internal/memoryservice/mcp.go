package memoryservice

import (
	"context"
	"errors"
	"strings"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/gluonfield/jazmem/pkg/jazmemhttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

const (
	PublicSearchToolName = "memory_search"
)

type AgenticSearchRequest struct {
	Query    string
	Deep     bool
	ParentID string
}

type AgenticSearcher interface {
	SearchMemory(context.Context, AgenticSearchRequest) (string, error)
}

func (s *Service) AddMCPTools(server *mcp.Server) {
	tools := memoryTools{service: s}
	mcp.AddTool(server, &mcp.Tool{
		Name:        PublicSearchToolName,
		Title:       "Search Jaz memory",
		Description: "Search Jaz memory through a delegated search worker. Returns an answer with useful references, checked pages, and search notes. Use before answering from memory; call memory_get_page only when raw markdown or edit context is needed.",
	}, tools.Search)
	jazmemhttp.AddMCPGetPageTool(server, gatedJazmem{service: s})
}

func (s *Service) RemoveMCPTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(PublicSearchToolName)
	}
	jazmemhttp.RemoveMCPGetPageTool(server)
}

func (s *Service) AddWorkerMCPTools(server *mcp.Server) {
	jazmemhttp.AddRawMCPTools(server, gatedJazmem{service: s})
}

func (s *Service) RemoveWorkerMCPTools(server *mcp.Server) {
	jazmemhttp.RemoveRawMCPTools(server)
}

func (s *Service) MCPToolsEnabled() bool {
	return s.Enabled()
}

func (s *Service) SetAgenticSearcher(searcher AgenticSearcher) {
	s.searcher = searcher
}

type memoryTools struct {
	service *Service
}

type gatedJazmem struct {
	service *Service
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"question or topic to answer from Jaz memory"`
	Deep  bool   `json:"deep,omitempty" jsonschema:"reserved for callers that know they need broader retrieval; the delegated search worker decides how to use raw deep search"`
}

func (t memoryTools) Search(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, nil, errors.New("query is required")
	}
	if err := t.ready(); err != nil {
		return nil, nil, err
	}
	if t.service.searcher == nil {
		return nil, nil, errors.New("memory agent is not configured")
	}
	answer, err := t.service.searcher.SearchMemory(ctx, AgenticSearchRequest{
		Query:    query,
		Deep:     input.Deep,
		ParentID: sessionIDFromRequest(req),
	})
	if err != nil {
		return nil, nil, err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "No memory answer was returned."
	}
	return textResult(answer)
}

func (t memoryTools) ready() error {
	if !t.service.Enabled() {
		return errors.New("memory is disabled in settings")
	}
	return nil
}

func (m gatedJazmem) Retrieve(ctx context.Context, query string, opts jazmem.SearchOptions) (jazmem.SearchResponse, error) {
	if err := m.ready(); err != nil {
		return jazmem.SearchResponse{}, err
	}
	return m.service.Memory.Retrieve(ctx, query, opts)
}

func (m gatedJazmem) GetPage(ctx context.Context, path string) (jazmem.Page, error) {
	if err := m.ready(); err != nil {
		return jazmem.Page{}, err
	}
	return m.service.Memory.GetPage(ctx, path)
}

func (m gatedJazmem) ready() error {
	if !m.service.Enabled() {
		return errors.New("memory is disabled in settings")
	}
	return nil
}

func textResult(texts ...string) (*mcp.CallToolResult, any, error) {
	content := make([]mcp.Content, 0, len(texts))
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text != "" {
			content = append(content, &mcp.TextContent{Text: text})
		}
	}
	return &mcp.CallToolResult{Content: content}, nil, nil
}

func sessionIDFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Extra == nil {
		return ""
	}
	return strings.TrimSpace(req.Extra.Header.Get(mcpsession.HeaderName))
}
