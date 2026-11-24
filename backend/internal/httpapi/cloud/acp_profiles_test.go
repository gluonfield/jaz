package cloud_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type fakeLocalACPExporter struct {
	exportResult acp.LocalAuthExport
	exportErr    error
	clearedPath  string
}

func (f *fakeLocalACPExporter) ExportLocalAuth(_ string) (acp.LocalAuthExport, error) {
	return f.exportResult, f.exportErr
}

func (f *fakeLocalACPExporter) ClearOwnedLocalAuth(_, storagePath string) error {
	f.clearedPath = storagePath
	return nil
}

func TestACPProfileHandlerProxyAndSyncLocal(t *testing.T) {
	var modelsCalls atomic.Int32
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"org_id": "org_1", "org_name": "Acme", "user_id": "u_1", "email": "dev@example.com", "role": "admin",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			n := modelsCalls.Add(1)
			models := []map[string]any{{"profile_id": "vertex-gemini", "provider": "vertex"}}
			if n > 1 {
				models = append(models, map[string]any{"profile_id": "acp:codex", "provider": "codex_oauth", "scope": "user"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/acp/profiles":
			_ = json.NewEncoder(w).Encode(map[string]any{"profiles": []any{}, "supported_agents": []any{}})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/acp/profiles/codex":
			var body struct {
				Files map[string]string `json:"files"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			raw, _ := base64.StdEncoding.DecodeString(body.Files["auth.json"])
			receivedAuth = string(raw)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profile": map[string]any{"agent": "codex", "profile_id": "acp:codex", "status": "connected", "version": 1},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	t.Cleanup(func() { _ = svc.Disconnect(context.Background(), "") })

	exporter := &fakeLocalACPExporter{
		exportResult: acp.LocalAuthExport{
			Agent:       acp.AgentCodex,
			Files:       map[string][]byte{"auth.json": []byte(`{"tokens":{"access_token":"tok"}}`)},
			StoragePath: "/tmp/jaz/acp/codex-home/auth.json",
			OwnedByJaz:  true,
		},
	}
	acpHandler := cloudapi.NewACPProfileHandler(svc, exporter)
	cloudHandler := cloudapi.NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", cloudHandler.ListModels)
	mux.HandleFunc("GET /v1/acp/profiles", acpHandler.Proxy)
	mux.HandleFunc("POST /v1/cloud/acp-profiles/{agent}/sync-local", acpHandler.SyncLocal)

	// Local mode rejects Cloud ACP proxy with 401.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/acp/profiles", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 in local mode, got %d", rec.Code)
	}

	// Connect to Cloud workspace.
	if _, err := svc.Connect(context.Background(), cloudsync.ConnectRequest{
		URL:   upstream.URL,
		Token: "tok-1",
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Prime /v1/models cache (1st call).
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "acp:codex") {
		t.Fatalf("unexpected initial models: %d %s", rec.Code, rec.Body.String())
	}

	// Proxy GET /v1/acp/profiles succeeds.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/acp/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/acp/profiles failed: %d %s", rec.Code, rec.Body.String())
	}

	// SyncLocal uploads auth.json, clears owned local credential, and invalidates /v1/models cache.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/acp-profiles/codex/sync-local", bytes.NewReader([]byte(`{"clear_local":true}`))))
	if rec.Code != http.StatusOK {
		t.Fatalf("SyncLocal failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(receivedAuth, "access_token") {
		t.Fatalf("expected uploaded auth.json, got %q", receivedAuth)
	}
	if exporter.clearedPath != "/tmp/jaz/acp/codex-home/auth.json" {
		t.Fatalf("expected owned local credential to be cleared, got %q", exporter.clearedPath)
	}
	if !strings.Contains(rec.Body.String(), `"cleared_local":true`) {
		t.Fatalf("expected cleared_local:true in response, got %s", rec.Body.String())
	}

	// /v1/models cache was invalidated so the next call fetches the newly connected acp:codex profile.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "acp:codex") {
		t.Fatalf("expected /v1/models cache invalidation to expose acp:codex, got %d %s", rec.Code, rec.Body.String())
	}
}
