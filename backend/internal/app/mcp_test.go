package app

import (
	"testing"

	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

func TestACPMCPServerReaderAlwaysIncludesProxyAndDirectJazTools(t *testing.T) {
	reader := acpMCPServerReader{
		authKey:     "internal-test-credential",
		proxyURL:    "http://127.0.0.1:5299/mcp/proxy",
		jaztoolsURL: "http://127.0.0.1:5299/mcp/jaztools",
	}

	servers, err := reader.ListMCPServers()
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
}
