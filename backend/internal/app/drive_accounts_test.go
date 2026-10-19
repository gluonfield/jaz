package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestExistingPersonalAndWorkspaceDriveAccountsRetainSeparateTools(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, account := range []integrations.Connection{
		{ID: "google_drive:personal", Provider: drive.ProviderID, AccountID: "personal@gmail.com", Alias: "personal"},
		{ID: "google_drive:work", Provider: drive.ProviderID, AccountID: "work@example.com", Alias: "work"},
	} {
		if err := store.SaveOAuthConnection(ctx, integrationoauth.Token{AccessToken: account.Alias, TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}, account); err != nil {
			t.Fatal(err)
		}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/files" {
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]string{{"id": "file", "name": r.Header.Get("Authorization")}}})
	}))
	defer upstream.Close()
	base := http.DefaultTransport
	target, _ := url.Parse(upstream.URL)
	http.DefaultTransport = driveTestTransport{base: base, upstream: target}
	t.Cleanup(func() {
		http.DefaultTransport = base
	})
	reader := connectionMCPServerReader{store: store, catalog: connections.NewCatalog()}
	manager := mcpruntime.NewManager(reader, store, tools.NewRegistry(), log.New(io.Discard))
	manager.Refresh(ctx)
	manager.Close()
	manager = mcpruntime.NewManager(reader, store, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 16 {
		t.Fatalf("account tool discovery = %+v, %v", list, err)
	}
	for _, alias := range []string{"personal", "work"} {
		result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "mcp_" + alias + "_search_files", Arguments: map[string]any{"pageSize": 1}})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(result.StructuredContent)
		var files drive.FileList
		if err := json.Unmarshal(data, &files); err != nil || result.IsError || len(files.Files) != 1 || files.Files[0].Title != "Bearer "+alias {
			t.Fatalf("%s search lost its account: %s, %v", alias, data, err)
		}
	}
	service := connections.NewService(connections.NewCatalog(), store, nil)
	if _, err := service.DisconnectAccount(ctx, "google_drive:personal"); err != nil {
		t.Fatal(err)
	}
	manager.Refresh(ctx)
	list, err = session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 8 {
		t.Fatalf("remaining work tools = %+v, %v", list, err)
	}
	for _, tool := range list.Tools {
		if len(tool.Name) < len("mcp_work_") || tool.Name[:len("mcp_work_")] != "mcp_work_" {
			t.Fatalf("disconnected account still visible: %s", tool.Name)
		}
	}
}
