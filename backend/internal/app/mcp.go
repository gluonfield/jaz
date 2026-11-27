package app

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/connections"
	driveconnector "github.com/wins/jaz/backend/internal/connectors/drive"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type acpMCPServerReader struct {
	proxyURL    string
	jaztoolsURL string
	cloudURL    string
	authKey     string
	loadSession func(string) (storage.Session, error)
}

func (r acpMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	return r.ListMCPServersForSession("")
}

func (r acpMCPServerReader) ListMCPServersForSession(sessionID string) ([]mcpconfig.Server, error) {
	servers := []mcpconfig.Server{mcpruntime.ProxyServerConfig(r.proxyURL), jaztools.ServerConfig(r.jaztoolsURL)}
	if sessionID != "" {
		session, err := r.loadSession(sessionID)
		if err != nil {
			return nil, err
		}
		if storage.NormalizeWorkspaceID(session.WorkspaceID) != storage.LocalWorkspaceID {
			servers = append(servers, mcpconfig.Server{
				ID: "jaz_cloud", Name: "jaz_cloud",
				Transport: mcpconfig.TransportStreamableHTTP,
				URL:       r.cloudURL, Enabled: true,
				Headers: []mcpconfig.Header{{Name: mcpsession.HeaderName, Value: mcpsession.HeaderPlaceholder}},
			})
		}
	}
	for i := range servers {
		servers[i].Headers = append(servers[i].Headers, mcpconfig.Header{Name: "Authorization", Value: "Bearer " + r.authKey})
	}
	return servers, nil
}

func NewACPMCPServerReader(jaz *jaztools.Service, urls serverconfig.URLs, key RuntimeAuthKey, store storage.Store) mcpconfig.ServerReader {
	return acpMCPServerReader{proxyURL: urls.MCPProxy, jaztoolsURL: jaz.URL(), cloudURL: urls.CloudMCP, authKey: string(key), loadSession: store.LoadSession}
}

// DeclareMCPServers applies JAZ_MCP_SERVERS before the MCP manager first
// connects, so a deployment starts with its servers wired.
func DeclareMCPServers(cfg Config, store *sqlitestore.Store) error {
	return mcpconfig.Declare(store, cfg.MCPServers)
}

func NewMCPManager(installed *plugins.Store, store *sqlitestore.Store, catalog *connections.Catalog, registry *tools.Registry, jaz *jaztools.Service, events *sessionevents.Bus, broker oauthbroker.Broker, cloud *cloudsync.Service, logger *log.Logger) *mcpruntime.Manager {
	reader := connectionMCPServerReader{store: store, catalog: catalog, plugins: installed, enabled: installed.IsEnabled, workspace: cloud.ActiveWorkspaceID}
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
	plugins   mcpconfig.ServerReader
	enabled   func(context.Context, string) bool
	store     connectionTokenStore
	catalog   *connections.Catalog
	workspace func() string
}

func (r connectionMCPServerReader) ListMCPServers() ([]mcpconfig.Server, error) {
	ctx := context.Background()
	ws := storage.LocalWorkspaceID
	if r.workspace != nil {
		ws = storage.NormalizeWorkspaceID(r.workspace())
	}
	var servers []mcpconfig.Server
	var (
		custom []mcpconfig.Server
		err    error
	)
	if wsStore, ok := r.store.(interface {
		ListMCPServersForWorkspace(string) ([]mcpconfig.Server, error)
	}); ok {
		custom, err = wsStore.ListMCPServersForWorkspace(ws)
	} else if ws == storage.LocalWorkspaceID {
		custom, err = r.store.ListMCPServers()
	}
	if err != nil {
		return nil, err
	}
	servers = append(servers, custom...)
	if r.plugins != nil {
		derived, err := r.plugins.ListMCPServers()
		if err != nil {
			return nil, err
		}
		servers = append(servers, derived...)
	}
	if ws != storage.LocalWorkspaceID {
		return servers, nil
	}
	for _, plugin := range r.catalog.ListPlugins() {
		if r.enabled != nil && !r.enabled(ctx, plugin.ID) {
			continue
		}
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
			Source:            mcpconfig.SourceConnection,
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
