package app

import (
	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

type acpMCPServerReader struct {
	base mcpconfig.ServerReader
	url  string
}

func (r acpMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	out, err := r.base.ListMCPServers()
	if err != nil {
		return nil, err
	}
	return append(out, jaztools.ServerConfig(r.url)), nil
}

func NewACPMCPServerReader(store *sqlitestore.Store, jaz *jaztools.Service) mcpconfig.ServerReader {
	return acpMCPServerReader{base: store, url: jaz.URL()}
}

func NewMCPManager(store *sqlitestore.Store, registry *tools.Registry, jaz *jaztools.Service, logger *log.Logger) *mcpruntime.Manager {
	return mcpruntime.NewManager(store, store, registry, logger, mcpruntime.WithBuiltinServerProvider(jaztools.ServerConfig(jaz.URL()), jaz.Server))
}
