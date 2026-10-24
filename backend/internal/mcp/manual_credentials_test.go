package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestEncryptedManualCredentialsAuthenticateMCPAfterReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "private-tools"}, nil)
	remote.AddTool(&mcpsdk.Tool{Name: "check", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: request.Extra.Header.Get("X-Key")}}}, nil
	})
	protocol := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return remote
	}, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer manual-key" || r.Header.Get("X-Key") != "manual-header" {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		protocol.ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)
	root := t.TempDir()
	store, err := sqlite.New(root)
	if err != nil {
		t.Fatal(err)
	}
	server, err := store.CreateMCPServer(mcpconfig.ServerInput{Name: "Private tools", URL: upstream.URL, Enabled: true, BearerToken: "manual-key", Headers: []mcpconfig.Header{{Name: "X-Key", Value: "manual-header"}}})
	t.Cleanup(func() {
		_ = store.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		manager := NewManager(store, store, tools.NewRegistry(), log.New(io.Discard))
		t.Cleanup(manager.Close)
		manager.Refresh(ctx)
		connection := manager.sessions[server.ID]
		if connection == nil {
			t.Fatalf("encrypted credentials failed to connect: %+v", manager.Status(server.ID))
		}
		result, err := connection.callTool(ctx, &mcpsdk.CallToolParams{Name: "check"})
		if err != nil || len(result.Content) != 1 || result.Content[0].(*mcpsdk.TextContent).Text != "manual-header" {
			t.Fatalf("persisted MCP credentials changed on pass %d: %v", pass, err)
		}
		manager.Close()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			store, err = sqlite.New(root)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
