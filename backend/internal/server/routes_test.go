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
