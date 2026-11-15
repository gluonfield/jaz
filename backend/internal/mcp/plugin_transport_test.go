package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestPluginRemoteTransportsUseHeadersAndRejectOriginChanges(t *testing.T) {
	for _, transport := range []string{mcpconfig.TransportStreamableHTTP, mcpconfig.TransportSSE} {
		t.Run(transport, func(t *testing.T) {
			remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "plugin-remote"}, nil)
			remote.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "connected"}}}, nil
			})
			var handler http.Handler = mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true})
			if transport == mcpconfig.TransportSSE {
				handler = mcpsdk.NewSSEHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil)
			}
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Plugin") != "fixture" || r.Method == http.MethodPost && r.Header.Get("Content-Type") == "invalid" {
					t.Error("configured header lost or protocol header overridden")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			defer endpoint.Close()
			config := mcpconfig.Server{ID: "plugin", Name: "Plugin", Source: mcpconfig.SourcePlugin, Transport: transport, URL: endpoint.URL, Enabled: true, Headers: []mcpconfig.Header{{Name: "X-Plugin", Value: "fixture"}, {Name: "Content-Type", Value: "invalid"}}}
			store := &testStore{servers: []mcpconfig.Server{config}}
			manager := NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
			defer manager.Close()
			manager.Refresh(t.Context())
			if status := manager.Status(config.ID); status.Status != "connected" || status.ToolCount != 1 {
				t.Fatalf("remote status = %+v", status)
			}
			result, err := manager.sessions[config.ID].callTool(t.Context(), &mcpsdk.CallToolParams{Name: "echo"})
			if err != nil || result.Content[0].(*mcpsdk.TextContent).Text != "connected" {
				t.Fatalf("remote call = %+v %v", result, err)
			}
			var forwarded atomic.Bool
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				forwarded.Store(true)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer target.Close()
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			}))
			defer redirect.Close()
			store.servers[0].URL = redirect.URL
			manager.Refresh(t.Context())
			if status := manager.Status(config.ID); status.Status != "error" || forwarded.Load() {
				t.Fatalf("cross-origin connection = %+v, forwarded=%v", status, forwarded.Load())
			}
		})
	}
}
