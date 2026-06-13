package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareKeepsHealthPublic(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
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

func TestAuthMiddlewareRejectsMissingKey(t *testing.T) {
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsBearerKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareAcceptsQueryKeyForSessionEvents(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/session-id/events?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyForWidgetContent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/widgets/widget-id/content?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestAuthMiddlewareRejectsQueryKeyOnMutations(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding?key=secret", nil)
	res := httptest.NewRecorder()
	(&Server{AuthKey: "secret"}).withAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}
