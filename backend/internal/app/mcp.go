package app

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

const jazToolsServerID = "jaztools"

type jazToolsServerReader struct {
	base mcpconfig.ServerReader
	url  string
}

func (r jazToolsServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	out, err := r.base.ListMCPServers()
	if err != nil {
		return nil, err
	}
	url := strings.TrimSpace(r.url)
	return append(out, mcpconfig.Server{
		ID:        jazToolsServerID,
		Name:      "jaztools",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       url,
		Enabled:   true,
	}), nil
}

func NewMCPServerReader(store *sqlitestore.Store, jaz *jaztools.Service) mcpconfig.ServerReader {
	return jazToolsServerReader{base: store, url: jaz.URL()}
}

func NewMCPManager(reader mcpconfig.ServerReader, store *sqlitestore.Store, registry *tools.Registry, jaz *jaztools.Service, logger *log.Logger) *mcpruntime.Manager {
	return mcpruntime.NewManager(reader, store, registry, logger, mcpruntime.WithLocalServerProvider(jazToolsServerID, jaz.Server))
}
