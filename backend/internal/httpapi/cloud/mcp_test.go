package cloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestNativeMCPSearchAndCallStayWithSessionWorkspace(t *testing.T) {
	ctx := t.Context()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var mu sync.Mutex
	var contexts []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer session-workspace-token" {
			t.Error("MCP request used a different workspace's token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": contexts})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/threads":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			hash, _ := body["source_id"].(string)
			if len(hash) != 64 || body["source_type"] != "native_mcp_context" || len(body) != 2 {
				t.Errorf("Cloud context received unexpected local data: %v", body)
			}
			body["id"] = "cloud-context"
			body["org_id"] = "org"
			body["owner_user_id"] = "owner"
			body["visibility"] = "private"
			contexts = append(contexts, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mcp":
			var req struct {
				ID     int
				Method string
				Params struct {
					Name      string
					Arguments struct{ Name string }
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var result string
			switch req.Method {
			case "tools/list":
				result = `{"tools":[{"name":"search_tools"},{"name":"call_tool"}]}`
			case "tools/call":
				if r.Header.Get("X-Jaz-Thread-ID") != "cloud-context" {
					t.Error("tool call missing its authorized Cloud thread")
					w.WriteHeader(http.StatusForbidden)
					return
				}
				switch {
				case req.Params.Name == "search_tools":
					result = `{"content":[{"type":"text","text":"workspace_custom_lookup"}]}`
				case req.Params.Name == "call_tool" && req.Params.Arguments.Name == "workspace_custom_lookup":
					result = `{"content":[{"type":"text","text":"lookup succeeded"}]}`
				default:
					t.Errorf("tool request changed at transport boundary: %+v", req)
				}
			}
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, result)
		default:
			t.Errorf("unexpected Cloud request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	for _, account := range []storage.CloudAccount{
		{ID: "session-workspace", BaseURL: upstream.URL, Token: "session-workspace-token", OrgID: "org", UserID: "owner"},
		{ID: "other-workspace", BaseURL: upstream.URL, Token: "other-workspace-token", OrgID: "org", UserID: "other"},
	} {
		if err := store.SaveCloudAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveActiveWorkspaceID(ctx, "other-workspace"); err != nil {
		t.Fatal(err)
	}
	native, err := store.CreateSession(storage.CreateSession{Title: "Private native session", WorkspaceID: "session-workspace"})
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.CreateSession(storage.CreateSession{WorkspaceID: storage.LocalWorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewMCPHandler(cloudsync.New(store, nil), store)
	post := func(sessionID, body string, status int, want string) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp/cloud", strings.NewReader(body))
		req.Header.Set(mcpsession.HeaderName, sessionID)
		req.Header.Set("X-Jaz-Thread-ID", "other-users-thread")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != status || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("MCP reply = %d %s, want %d with %q", rec.Code, rec.Body.String(), status, want)
		}
	}
	post(local.ID, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, http.StatusForbidden, "native Cloud-workspace")
	post(native.ID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, http.StatusAccepted, "")
	post(native.ID, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, http.StatusOK, "search_tools")
	post(native.ID, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_tools","arguments":{"query":"lookup"}}}`, http.StatusOK, "workspace_custom_lookup")
	handler = NewMCPHandler(cloudsync.New(store, nil), store)
	post(native.ID, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"workspace_custom_lookup","arguments":{}}}}`, http.StatusOK, "lookup succeeded")
	mu.Lock()
	defer mu.Unlock()
	if len(contexts) != 1 {
		t.Fatalf("native session created %d Cloud contexts", len(contexts))
	}
}
