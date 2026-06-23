package threads

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const MCPToolThreadContext = "thread_context"

func (s *Service) AddMCPTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        MCPToolThreadContext,
		Title:       "Get thread context",
		Description: "Return compact bounded context from another Jaz thread. With query, returns matching message neighborhoods; with before_seq, after_seq, or around_seq, returns a page; otherwise returns the latest messages. It does not expose raw local files or full transcript dumps.",
	}, s.ContextMCP)
}

func (s *Service) ContextMCP(ctx context.Context, _ *mcp.CallToolRequest, input ContextRequest) (*mcp.CallToolResult, ContextResponse, error) {
	response, err := s.Context(ctx, input)
	return nil, response, err
}
