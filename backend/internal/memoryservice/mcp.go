package memoryservice

import (
	"context"
	"errors"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/gluonfield/jazmem/pkg/jazmemhttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Service) AddMCPTools(server *mcp.Server) {
	jazmemhttp.AddMCPTools(server, gatedMemory{s})
}

func (s *Service) RemoveMCPTools(server *mcp.Server) {
	jazmemhttp.RemoveMCPTools(server)
}

func (s *Service) MCPToolsEnabled() bool {
	return s.Enabled()
}

type gatedMemory struct {
	service *Service
}

func (m gatedMemory) AgenticSearch(ctx context.Context, query string, opts jazmem.AgenticOptions) (jazmem.AgenticResponse, error) {
	if err := m.ready(); err != nil {
		return jazmem.AgenticResponse{}, err
	}
	return m.service.Memory.AgenticSearch(ctx, query, opts)
}

func (m gatedMemory) Retrieve(ctx context.Context, query string, opts jazmem.SearchOptions) (jazmem.SearchResponse, error) {
	if err := m.ready(); err != nil {
		return jazmem.SearchResponse{}, err
	}
	return m.service.Memory.Retrieve(ctx, query, opts)
}

func (m gatedMemory) GetPage(ctx context.Context, slug string) (jazmem.Page, error) {
	if err := m.ready(); err != nil {
		return jazmem.Page{}, err
	}
	return m.service.Memory.GetPage(ctx, slug)
}

func (m gatedMemory) ready() error {
	if !m.service.Enabled() {
		return errors.New("memory is disabled in settings")
	}
	return nil
}
