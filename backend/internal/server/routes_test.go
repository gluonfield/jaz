package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerMountsInjectedRoutes(t *testing.T) {
	srv := &Server{Routes: Routes{{
		Pattern: "GET /v1/test-route",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}}}
	res := httptest.NewRecorder()

	srv.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/test-route", nil))

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestPublicRouteHandlesRequestBeforeAPIMux(t *testing.T) {
	srv := &Server{
		AuthKey: "secret",
		PublicRoutes: PublicRoutes{{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
			Match: func(r *http.Request) bool {
				return r.Host == "jaz-preview-abc123.localhost:5299"
			},
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
	req.Host = "jaz-preview-abc123.localhost:5299"
	res := httptest.NewRecorder()

	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusTeapot {
		t.Fatalf("status = %d", res.Code)
	}
}
