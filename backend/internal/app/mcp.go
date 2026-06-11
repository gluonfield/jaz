package app

import (
	"strings"

	"github.com/charmbracelet/log"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/memoryservice"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

const memoryMCPServerID = "jazmem"

type memoryMCPSource interface {
	Enabled() bool
	MCPURL() string
}

type memoryMCPServerReader struct {
	base   mcpconfig.ServerReader
	memory memoryMCPSource
}

func (r memoryMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	var out []mcpconfig.Server
	if r.base != nil {
		servers, err := r.base.ListMCPServers()
		if err != nil {
			return nil, err
		}
		out = append(out, servers...)
	}
	if r.memory == nil || !r.memory.Enabled() {
		return out, nil
	}
	url := strings.TrimSpace(r.memory.MCPURL())
	if url == "" {
		return out, nil
	}
	return append(out, mcpconfig.Server{
		ID:        memoryMCPServerID,
		Name:      "jazmem",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       url,
		Enabled:   true,
	}), nil
}

func NewMCPServerReader(store *sqlitestore.Store, memory *memoryservice.Service) mcpconfig.ServerReader {
	return memoryMCPServerReader{base: store, memory: memory}
}

func NewMCPManager(reader mcpconfig.ServerReader, store *sqlitestore.Store, registry *tools.Registry, memory *memoryservice.Service, logger *log.Logger) *mcpruntime.Manager {
	return mcpruntime.NewManager(reader, store, registry, logger, mcpruntime.WithLocalServer(memoryMCPServerID, memory.MCPServer()))
}
