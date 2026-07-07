package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type proxyRuntime struct {
	called bool
}

func (r *proxyRuntime) Refresh(context.Context) {}

func (r *proxyRuntime) Status(string) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *proxyRuntime) Test(context.Context, mcpconfig.Server) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *proxyRuntime) Authorize(context.Context, mcpconfig.Server, mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	return mcpconfig.ServerStatus{}
}

func (r *proxyRuntime) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		r.called = true
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestMCPProxyHandlerRequiresSessionHeader(t *testing.T) {
	runtime := &proxyRuntime{}
	handler := (&Server{MCP: runtime}).mcpProxyHandler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp/proxy", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if runtime.called {
		t.Fatal("proxy runtime was called without a session header")
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp/proxy", nil)
	req.Header.Set(mcpsession.HeaderName, "session-1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
