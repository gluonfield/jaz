package acp_test

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type staticMCPStore struct {
	servers []mcpconfig.Server
}

func (s staticMCPStore) ListMCPServers() ([]mcpconfig.Server, error) {
	return append([]mcpconfig.Server(nil), s.servers...), nil
}

func TestManagerPassesEnabledHTTPMCPServersToCapableACPAgent(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := acp.NewManager(store, acp.Config{
		Root:      t.TempDir(),
		Workspace: t.TempDir(),
		MCPStore: staticMCPStore{servers: []mcpconfig.Server{
			mcpruntime.ProxyServerConfig("http://127.0.0.1:5299/mcp/proxy"),
			jaztools.ServerConfig("http://127.0.0.1:5299/mcp/jaztools"),
		}},
		Agents: map[string]acp.AgentConfig{
			"fake": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":      "1",
					"JAZ_FAKE_ACP_MCP_HTTP":   "1",
					"JAZ_FAKE_ACP_EXPECT_MCP": "1",
				},
			},
		},
	}, log.New(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()
}
