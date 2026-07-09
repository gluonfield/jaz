package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
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

func NewACPMCPServerReader(store *sqlitestore.Store, catalog *connections.Catalog, jaz *jaztools.Service, urls serverconfig.URLs) mcpconfig.ServerReader {
	return acpMCPServerReader{base: connectionMCPServerReader{store: store, catalog: catalog}, proxyURL: urls.MCPProxy, jaztoolsURL: jaz.URL()}
}

func NewMCPManager(store *sqlitestore.Store, catalog *connections.Catalog, registry *tools.Registry, jaz *jaztools.Service, logger *log.Logger) *mcpruntime.Manager {
	reader := connectionMCPServerReader{store: store, catalog: catalog}
	return mcpruntime.NewManager(reader, store, registry, logger, mcpruntime.WithBuiltinServerProvider(jaztools.ServerConfig(jaz.URL()), jaz.Server))
}

type connectionTokenStore interface {
	mcpconfig.ServerReader
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	LoadToken(context.Context, string) (integrationoauth.Token, bool, error)
}

type connectionMCPServerReader struct {
	store   connectionTokenStore
	catalog *connections.Catalog
}

func (r connectionMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	servers, err := r.store.ListMCPServers()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	for _, plugin := range r.catalog.ListPlugins() {
		remote := plugin.RemoteMCP
		if remote == nil || !plugin.UsesConnectionMCP() {
			continue
		}
		backed, err := r.connectionBackedServers(ctx, plugin, remote.URL)
		if err != nil {
			return nil, err
		}
		servers = append(servers, backed...)
	}
	return servers, nil
}

func (r connectionMCPServerReader) connectionBackedServers(ctx context.Context, plugin integrations.Plugin, url string) ([]mcpconfig.Server, error) {
	providerID := plugin.Provider.ID
	if providerID == "" {
		providerID = plugin.ID
	}
	accounts, err := r.store.ListConnections(ctx, providerID)
	if err != nil {
		return nil, err
	}
	var out []mcpconfig.Server
	for _, account := range accounts {
		if plugin.PrimaryAuthKind() == integrations.AuthKindMCPConnection {
			out = append(out, mcpconfig.Server{
				ID:        account.ID,
				Name:      remoteServerName(account),
				Transport: mcpconfig.TransportStreamableHTTP,
				URL:       url,
				Enabled:   true,
			})
			continue
		}
		token, ok, err := r.store.LoadToken(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		if !ok || token.AccessToken == "" {
			continue
		}
		out = append(out, mcpconfig.Server{
			ID:        account.ID,
			Name:      remoteServerName(account),
			Transport: mcpconfig.TransportStreamableHTTP,
			URL:       url,
			Enabled:   true,
			Headers:   []mcpconfig.Header{{Name: "Authorization", Value: "Bearer " + token.AccessToken}},
		})
	}
	return out, nil
}

func remoteServerName(account integrations.Connection) string {
	if ref := account.AccountRef(); ref != "" {
		return ref
	}
	return account.Provider
}
