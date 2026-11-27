package vault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	vaultcore "github.com/wins/jaz/backend/internal/vault"
)

type stubCloudProxy struct {
	intercept bool
	calls     int
}

func (p *stubCloudProxy) ProxyActiveWorkspace(w http.ResponseWriter, _ *http.Request) bool {
	if !p.intercept {
		return false
	}
	p.calls++
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"proxied":true}`))
	return true
}

func newTestMux(t *testing.T, cloud CloudProxy) *http.ServeMux {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	h := NewHandler(vaultcore.NewService(store))
	if cloud != nil {
		h.WithCloud(cloud)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/vault/items", h.List)
	mux.HandleFunc("POST /v1/vault/items", h.Create)
	mux.HandleFunc("PATCH /v1/vault/items/{id}", h.Update)
	mux.HandleFunc("DELETE /v1/vault/items/{id}", h.Delete)
	return mux
}

func TestVaultHTTPHandlerCRUDNeverLeaksSecrets(t *testing.T) {
	mux := newTestMux(t, nil)

	createBody := `{"kind":"login","label":"GitHub","origins":["https://github.com"],"username":"octocat","password":"top-secret-password","totp_secret":"JBSWY3DPEHPK3PXP"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/vault/items", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "top-secret-password") || strings.Contains(rec.Body.String(), "JBSWY3DPEHPK3PXP") {
		t.Fatalf("create response leaked secret: %s", rec.Body.String())
	}
	var created struct {
		Item storage.VaultItem `json:"item"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Item.ID == "" || !created.Item.HasTOTP {
		t.Fatalf("unexpected created item: %+v", created.Item)
	}

	// List with domain filter
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/v1/vault/items?kind=login&domain=github.com", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed: %d %s", listRec.Code, listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), "top-secret-password") {
		t.Fatalf("list response leaked secret")
	}

	// Patch label without wiping password
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/vault/items/"+created.Item.ID, strings.NewReader(`{"label":"GitHub Personal"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	mux.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK || !strings.Contains(patchRec.Body.String(), "GitHub Personal") {
		t.Fatalf("patch failed: %d %s", patchRec.Code, patchRec.Body.String())
	}

	// Delete + 404 on repeat
	delRec := httptest.NewRecorder()
	mux.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/v1/vault/items/"+created.Item.ID, nil))
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", delRec.Code, delRec.Body.String())
	}
	delAgain := httptest.NewRecorder()
	mux.ServeHTTP(delAgain, httptest.NewRequest(http.MethodDelete, "/v1/vault/items/"+created.Item.ID, nil))
	if delAgain.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on second delete, got %d", delAgain.Code)
	}
}

func TestVaultHTTPHandlerProxiesToCloudWorkspace(t *testing.T) {
	proxy := &stubCloudProxy{intercept: true}
	mux := newTestMux(t, proxy)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/vault/items"},
		{http.MethodPost, "/v1/vault/items"},
		{http.MethodPatch, "/v1/vault/items/vlt_1"},
		{http.MethodDelete, "/v1/vault/items/vlt_1"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"proxied":true`) {
			t.Fatalf("%s %s was not proxied to cloud: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if proxy.calls != 4 {
		t.Fatalf("expected 4 proxied calls, got %d", proxy.calls)
	}
}
