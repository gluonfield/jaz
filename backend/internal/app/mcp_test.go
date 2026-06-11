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

type testMemoryMCP struct {
	enabled bool
	url     string
}

func (m testMemoryMCP) Enabled() bool  { return m.enabled }
func (m testMemoryMCP) MCPURL() string { return m.url }

func TestMemoryMCPServerReaderAppendsEnabledMemory(t *testing.T) {
	reader := memoryMCPServerReader{
		base: testMCPReader{servers: []mcpconfig.Server{{
			ID:      "docs",
			Name:    "Docs",
			URL:     "https://docs.example.com/mcp",
			Enabled: true,
		}}},
		memory: testMemoryMCP{enabled: true, url: "http://127.0.0.1:5299/mcp/jazmem"},
	}

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("server count = %d, want 2", len(servers))
	}
	memory := servers[1]
	if memory.ID != memoryMCPServerID || memory.Name != "jazmem" ||
		memory.Transport != mcpconfig.TransportStreamableHTTP ||
		memory.URL != "http://127.0.0.1:5299/mcp/jazmem" || !memory.Enabled {
		t.Fatalf("memory server = %#v", memory)
	}
}

func TestMemoryMCPServerReaderSkipsDisabledMemory(t *testing.T) {
	reader := memoryMCPServerReader{
		base:   testMCPReader{},
		memory: testMemoryMCP{enabled: false, url: "http://127.0.0.1:5299/mcp/jazmem"},
	}

	servers, err := reader.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("servers = %#v, want none", servers)
	}
}

func TestMemoryMCPServerReaderReturnsBaseError(t *testing.T) {
	want := errors.New("load failed")
	reader := memoryMCPServerReader{base: testMCPReader{err: want}}

	if _, err := reader.ListMCPServers(); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
