package app

import (
	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

type acpMCPServerReader struct {
	base        mcpconfig.ServerReader
	proxyURL    string
	jaztoolsURL string
}

func (r acpMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	servers, err := r.base.ListMCPServers()
	if err != nil {
		return nil, err
	}
	out := []mcpconfig.Server{}
	if hasEnabledUserMCPServer(servers) && r.proxyURL != "" {
		out = append(out, mcpruntime.ProxyServerConfig(r.proxyURL))
	}
	return append(out, jaztools.ServerConfig(r.jaztoolsURL)), nil
}

func hasEnabledUserMCPServer(servers []mcpconfig.Server) bool {
	for _, server := range servers {
		if server.Enabled {
			return true
		}
	}
	return false
}

func NewACPMCPServerReader(store *sqlitestore.Store, jaz *jaztools.Service, urls serverconfig.URLs) mcpconfig.ServerReader {
	return acpMCPServerReader{base: store, proxyURL: urls.MCPProxy, jaztoolsURL: jaz.URL()}
}

func NewMCPManager(store *sqlitestore.Store, registry *tools.Registry, jaz *jaztools.Service, logger *log.Logger) *mcpruntime.Manager {
	return mcpruntime.NewManager(store, store, registry, logger, mcpruntime.WithBuiltinServerProvider(jaztools.ServerConfig(jaz.URL()), jaz.Server))
}
