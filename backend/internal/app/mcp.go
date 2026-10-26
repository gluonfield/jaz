package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/connections"
	driveconnector "github.com/wins/jaz/backend/internal/connectors/drive"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type acpMCPServerReader struct {
	proxyURL    string
	jaztoolsURL string
	authKey     string
}

func (r acpMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	servers := []mcpconfig.Server{mcpruntime.ProxyServerConfig(r.proxyURL), jaztools.ServerConfig(r.jaztoolsURL)}
	for i := range servers {
		servers[i].Headers = append(servers[i].Headers, mcpconfig.Header{Name: "Authorization", Value: "Bearer " + r.authKey})
	}
	return servers, nil
}

func NewACPMCPServerReader(jaz *jaztools.Service, urls serverconfig.URLs, key RuntimeAuthKey) mcpconfig.ServerReader {
	return acpMCPServerReader{proxyURL: urls.MCPProxy, jaztoolsURL: jaz.URL(), authKey: string(key)}
}

// DeclareMCPServers applies JAZ_MCP_SERVERS before the MCP manager first
// connects, so a deployment starts with its servers wired.
func DeclareMCPServers(cfg Config, store *sqlitestore.Store) error {
	return mcpconfig.Declare(store, cfg.MCPServers)
}

func NewMCPManager(store *sqlitestore.Store, catalog *connections.Catalog, registry *tools.Registry, jaz *jaztools.Service, events *sessionevents.Bus, broker oauthbroker.Broker, logger *log.Logger) *mcpruntime.Manager {
	reader := connectionMCPServerReader{store: store, catalog: catalog}
	return mcpruntime.NewManager(reader, store, registry, logger,
		mcpruntime.WithBuiltinServerProvider(jaztools.ServerConfig(jaz.URL()), jaz.Server),
		mcpruntime.WithSessionEvents(store, events),
		mcpruntime.WithOAuthBroker(broker))
}

type connectionTokenStore interface {
	mcpconfig.ServerReader
	integrationoauth.Store
	ListConnections(context.Context, string) ([]integrations.Connection, error)
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
		if !plugin.UsesConnectionMCP() {
			continue
		}
		var endpoint string
		if remote != nil {
			endpoint = remote.URL
		}
		backed, err := r.connectionBackedServers(ctx, plugin, endpoint)
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
		server := mcpconfig.Server{
			ID:                account.ID,
			Name:              remoteServerName(account),
			Transport:         mcpconfig.TransportStreamableHTTP,
			URL:               url,
			Enabled:           true,
			TokenConnectionID: account.ID,
		}
		if plugin.Provider.ID == driveconnector.ProviderID {
			server.Transport = mcpconfig.TransportInProcess
			server.LocalServer = connections.NewDriveMCPServer(r.store, account.ID)
		}
		out = append(out, server)
	}
	return out, nil
}

func remoteServerName(account integrations.Connection) string {
	if ref := account.AccountRef(); ref != "" {
		return ref
	}
	return account.Provider
}
