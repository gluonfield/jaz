package app

import (
	"errors"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

type testMCPReader struct {
	servers []mcpconfig.Server
	err     error
}

func (r testMCPReader) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), r.servers...), r.err
}

func TestJazToolsServerReaderAppendsManagedServer(t *testing.T) {
	reader := jazToolsServerReader{
		base: testMCPReader{servers: []mcpconfig.Server{{
			ID:      "docs",
			Name:    "Docs",
			URL:     "https://docs.example.com/mcp",
			Enabled: true,
		}}},
		url: "http://127.0.0.1:5299/mcp/jaztools",
	}

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("server count = %d, want 2", len(servers))
	}
	jaz := servers[1]
	if jaz.ID != jazToolsServerID || jaz.Name != "jaztools" ||
		jaz.Transport != mcpconfig.TransportStreamableHTTP ||
		jaz.URL != "http://127.0.0.1:5299/mcp/jaztools" || !jaz.Enabled {
		t.Fatalf("jaz server = %#v", jaz)
	}
}

func TestJazToolsServerReaderReturnsBaseError(t *testing.T) {
	want := errors.New("load failed")
	reader := jazToolsServerReader{
		base: testMCPReader{err: want},
		url:  "http://127.0.0.1:5299/mcp/jaztools",
	}

	if _, err := reader.ListMCPServers(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
