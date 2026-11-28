package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
)

func TestAuthMiddlewareKeepsHealthPublic(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		OK           bool `json:"ok"`
		AuthRequired bool `json:"auth_required"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.AuthRequired {
		t.Fatalf("health = %#v", body)
	}
}

func TestAuthMiddlewareKeepsWellKnownDiscoveryPublic(t *testing.T) {
	for _, tc := range []struct {
		name     string
		authKey  string
		wantAuth string
	}{
		{"with auth key", "secret", "bearer"},
		{"without auth key", "", "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: tc.authKey}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/.well-known/jaz-server", nil))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
			var body serverDiscoveryResponse
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !body.OK || body.Mode != "local" || body.Auth != tc.wantAuth || !body.Capabilities.SessionFileRead {
				t.Fatalf("discovery = %#v, want auth=%q", body, tc.wantAuth)
			}
		})
	}
}

func TestAuthMiddlewareRejectsMissingKey(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsBearerKeyAndIssuesResourceTicket(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		OK             bool            `json:"ok"`
		AuthKind       string          `json:"auth_kind"`
		ResourceTicket *ResourceTicket `json:"resource_ticket"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.AuthKind != "bearer" || body.ResourceTicket == nil || body.ResourceTicket.Token == "" {
		t.Fatalf("auth check = %#v", body)
	}
}

func TestAuthMiddlewareRejectsQueryKeyForSessionFileJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-id/file?path=paper.pdf&key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyOnMutations(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsLocalBrowserExtensionWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/thread/browser", nil)
	req.RemoteAddr = "127.0.0.1:53124"
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnopabcdefghijklmnop")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsWebsiteOriginBrowserExtensionWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/thread/browser", nil)
	req.RemoteAddr = "127.0.0.1:53124"
	req.Header.Set("Origin", "https://example.com")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsMalformedChromeExtensionOriginWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/thread/browser", nil)
	req.RemoteAddr = "127.0.0.1:53124"
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsRemoteBrowserExtensionWithoutKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/thread/browser", nil)
	req.RemoteAddr = "203.0.113.10:53124"
	req.Header.Set("Origin", "chrome-extension://abcdefghijklmnopabcdefghijklmnop")
	res := httptest.NewRecorder()
	(&Server{ModelCatalog: modelcatalog.NewService(nil), AuthKey: "root-key"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}
