package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestSavedMCPRetryRecoversLiveToolsWithoutCheckingOtherServers(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote"}, nil)
	remote.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "recovered"}}}, nil
	})
	var available atomic.Bool
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	var unrelatedRequests atomic.Int32
	unrelated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unrelatedRequests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer unrelated.Close()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := store.CreateMCPServer(mcpconfig.ServerInput{Name: "remote", URL: upstream.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateMCPServer(mcpconfig.ServerInput{Name: "unrelated", URL: unrelated.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	manager := mcpruntime.NewManager(store, nil, registry, log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	if status := manager.Status(server.ID); status.Status != "error" {
		t.Fatalf("initial status = %#v", status)
	}
	before := unrelatedRequests.Load()
	otherStatus := manager.Status(other.ID)
	available.Store(true)
	handler := (&Server{ModelCatalog: modelcatalog.NewService(nil), Store: store, MCP: manager}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+server.ID+"/test", nil))
	var status mcpconfig.ServerStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || response.Code != http.StatusOK || status.Status != "connected" {
		t.Fatalf("retry = %d %s, %v", response.Code, response.Body.String(), err)
	}
	if status := manager.Status(server.ID); status.Status != "connected" || status.ToolCount != 1 {
		t.Fatalf("retry left stale live status = %#v", status)
	}
	if unrelatedRequests.Load() != before {
		t.Fatal("retry contacted an unrelated server")
	}
	if status := manager.Status(other.ID); status.Status != otherStatus.Status || status.Error != otherStatus.Error || !status.CheckedAt.Equal(otherStatus.CheckedAt) {
		t.Fatalf("retry changed unrelated status = %#v", status)
	}
	response = httptest.NewRecorder()
	draft := strings.NewReader(`{"name":"draft","url":"` + unrelated.URL + `"}`)
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+server.ID+"/test", draft))
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || response.Code != http.StatusOK || status.Status != "error" {
		t.Fatalf("draft probe = %d %s, %v", response.Code, response.Body.String(), err)
	}
	if status := manager.Status(server.ID); status.Status != "connected" {
		t.Fatalf("draft probe changed the saved connection = %#v", status)
	}
	for _, body := range []string{`{`, `{"name":"invalid","url":"bad-url"}`} {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/mcp/servers/"+server.ID+"/test", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid draft probed saved settings: %d %s", response.Code, response.Body.String())
		}
	}
	definitions := registry.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("live tools = %d, want 1", len(definitions))
	}
	tool, ok := registry.Get(tools.DefinitionName(definitions[0]))
	if !ok {
		t.Fatal("recovered tool is unavailable")
	}
	if _, err := tool.Execute(t.Context(), nil); err != nil {
		t.Fatalf("recovered live tool failed: %v", err)
	}
}
