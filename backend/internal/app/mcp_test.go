package app

import (
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/jaztools"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type testMCPReader struct {
	servers []mcpconfig.Server
	err     error
}

func (r testMCPReader) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), r.servers...), r.err
}

func TestACPMCPServerReaderAppendsJazTools(t *testing.T) {
	reader := acpMCPServerReader{
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
	if jaz.ID != jaztools.ServerID || jaz.Name != jaztools.ServerName ||
		jaz.Transport != mcpconfig.TransportStreamableHTTP ||
		jaz.URL != "http://127.0.0.1:5299/mcp/jaztools" || !jaz.Enabled {
		t.Fatalf("jaz server = %#v", jaz)
	}
	if len(jaz.Headers) != 1 || jaz.Headers[0].Name != mcpsession.HeaderName || jaz.Headers[0].Value != mcpsession.HeaderPlaceholder {
		t.Fatalf("jaz headers = %#v", jaz.Headers)
	}
}

func TestACPMCPServerReaderReturnsBaseError(t *testing.T) {
	want := errors.New("load failed")
	reader := acpMCPServerReader{
		base: testMCPReader{err: want},
		url:  "http://127.0.0.1:5299/mcp/jaztools",
	}

	if _, err := reader.ListMCPServers(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
