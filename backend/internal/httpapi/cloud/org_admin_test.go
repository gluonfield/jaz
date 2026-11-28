package cloud_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestOrgAdminProxyIdentityAndIcon(t *testing.T) {
	var adminRequest string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id": "u1", "org_id": "org_a", "org_name": "Acme", "email": "dev@acme.test", "role": "admin",
				"org_icon":        map[string]any{"kind": "image", "url": "/v1/org/icon?v=abc"},
				"resource_ticket": "cloud-resource-ticket",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/admin/api-keys/integration-acme":
			body, _ := io.ReadAll(r.Body)
			adminRequest = r.Header.Get("Authorization") + " " + string(body)
			_ = json.NewEncoder(w).Encode(map[string]any{"key_id": "integration-acme", "configured": true, "last4": "wxyz"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/org/icon":
			if r.URL.Query().Has("ticket") {
				t.Errorf("local resource ticket leaked upstream: %s", r.URL.RawQuery)
			}
			w.Header().Set("ETag", `"abc"`)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			if r.Header.Get("If-None-Match") == `"abc"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
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
	cloudSvc := cloudsync.New(store, nil)
	if _, err := cloudSvc.Connect(t.Context(), cloudsync.ConnectRequest{URL: upstream.URL, Token: "cloud-jwt"}); err != nil {
		t.Fatal(err)
	}
	h := cloudapi.NewHandler(cloudSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/auth/me", h.Me)
	mux.HandleFunc("/v1/admin/", h.Proxy)
	mux.HandleFunc("GET /v1/org/icon", h.Proxy)
	serve := func(method, target, body string, header http.Header) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		for k, v := range header {
			req.Header[k] = v
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	me := serve(http.MethodGet, "/v1/auth/me", "", nil)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"org_icon":{"kind":"image","url":"/v1/org/icon?v=abc"}`) {
		t.Fatalf("GET /v1/auth/me status=%d body=%s", me.Code, me.Body.String())
	}
	if strings.Contains(me.Body.String(), "cloud-resource-ticket") {
		t.Fatalf("cloud resource ticket relayed to the client: %s", me.Body.String())
	}

	put := serve(http.MethodPut, "/v1/admin/api-keys/integration-acme", `{"api_key":"sk-secret-wxyz"}`, nil)
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"last4":"wxyz"`) {
		t.Fatalf("PUT admin api key status=%d body=%s", put.Code, put.Body.String())
	}
	if adminRequest != `Bearer cloud-jwt {"api_key":"sk-secret-wxyz"}` {
		t.Fatalf("upstream admin request = %q", adminRequest)
	}

	icon := serve(http.MethodGet, "/v1/org/icon?v=abc&ticket=local", "", nil)
	if icon.Code != http.StatusOK || icon.Body.String() != "png-bytes" || icon.Header().Get("Content-Type") != "image/png" ||
		icon.Header().Get("X-Content-Type-Options") != "nosniff" || icon.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatalf("GET /v1/org/icon status=%d headers=%v body=%s", icon.Code, icon.Header(), icon.Body.String())
	}
	cached := serve(http.MethodGet, "/v1/org/icon?v=abc", "", http.Header{"If-None-Match": {`"abc"`}})
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Fatalf("conditional GET /v1/org/icon status=%d body=%s", cached.Code, cached.Body.String())
	}

	if _, err := cloudSvc.SwitchWorkspace(t.Context(), cloudsync.LocalWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if local := serve(http.MethodGet, "/v1/admin/org", "", nil); local.Code != http.StatusUnauthorized {
		t.Fatalf("GET /v1/admin/org in Local mode status=%d, want 401", local.Code)
	}
}
