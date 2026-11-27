package app

import (
	"testing"

	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestACPMCPServerReaderAlwaysIncludesProxyAndDirectJazTools(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	local, err := store.CreateSession(storage.CreateSession{WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	native, err := store.CreateSession(storage.CreateSession{WorkspaceID: "cloud-workspace"})
	if err != nil {
		t.Fatal(err)
	}
	reader := acpMCPServerReader{
		authKey:     "internal-test-credential",
		proxyURL:    "http://127.0.0.1:5299/mcp/proxy",
		jaztoolsURL: "http://127.0.0.1:5299/mcp/jaztools",
		cloudURL:    "http://127.0.0.1:5299/mcp/cloud",
		loadSession: store.LoadSession,
	}

	servers, err := reader.ListMCPServersForSession(local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("server count = %d, want 2", len(servers))
	}
	proxy := servers[0]
	if proxy.ID != mcpruntime.ProxyServerID || proxy.Name != mcpruntime.ProxyServerName ||
		proxy.Transport != mcpconfig.TransportStreamableHTTP ||
		proxy.URL != "http://127.0.0.1:5299/mcp/proxy" || !proxy.Enabled {
		t.Fatalf("proxy server = %#v", proxy)
	}
	if len(proxy.Headers) != 2 || proxy.Headers[0].Name != mcpsession.HeaderName || proxy.Headers[0].Value != mcpsession.HeaderPlaceholder {
		t.Fatalf("proxy headers = %#v", proxy.Headers)
	}
	if proxy.Headers[1].Name != "Authorization" || proxy.Headers[1].Value != "Bearer internal-test-credential" {
		t.Fatal("proxy has no internal credential")
	}
	jaz := servers[1]
	if jaz.ID != jaztools.ServerID || jaz.Name != jaztools.ServerName ||
		jaz.Transport != mcpconfig.TransportStreamableHTTP ||
		jaz.URL != "http://127.0.0.1:5299/mcp/jaztools" || !jaz.Enabled {
		t.Fatalf("jaz server = %#v", jaz)
	}
	if len(jaz.Headers) != 2 || jaz.Headers[0].Name != mcpsession.HeaderName || jaz.Headers[0].Value != mcpsession.HeaderPlaceholder {
		t.Fatalf("jaz headers = %#v", jaz.Headers)
	}
	servers, err = reader.ListMCPServersForSession(native.ID)
	if err != nil || len(servers) != 3 {
		t.Fatalf("native Cloud-workspace MCP servers = %+v, err = %v", servers, err)
	}
	cloud := servers[2]
	if cloud.Name != "jaz_cloud" || cloud.URL != "http://127.0.0.1:5299/mcp/cloud" || !cloud.Enabled {
		t.Fatalf("Cloud MCP server = %#v", cloud)
	}
	if len(cloud.Headers) != 2 || cloud.Headers[0] != (mcpconfig.Header{Name: mcpsession.HeaderName, Value: mcpsession.HeaderPlaceholder}) ||
		cloud.Headers[1] != (mcpconfig.Header{Name: "Authorization", Value: "Bearer internal-test-credential"}) {
		t.Fatalf("Cloud MCP headers = %#v", cloud.Headers)
	}
}
