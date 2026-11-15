package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestSelectiveRefreshPreservesOtherServerSessionAndTools(t *testing.T) {
	var fail atomic.Bool
	echo := newEchoHandler()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "target unavailable", http.StatusServiceUnavailable)
			return
		}
		echo.ServeHTTP(w, r)
	}))
	defer target.Close()
	var otherRequests atomic.Int32
	otherEcho := newEchoHandler()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherRequests.Add(1)
		otherEcho.ServeHTTP(w, r)
	}))
	defer other.Close()
	registry := tools.NewRegistry()
	manager := NewManager(&testStore{servers: []mcpconfig.Server{
		{ID: "target", Name: "Target", URL: target.URL, Enabled: true},
		{ID: "other", Name: "Other", URL: other.URL, Enabled: true},
	}}, nil, registry, log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	if count := len(registry.Definitions()); count != 2 {
		t.Fatalf("initial tools = %d", count)
	}
	original := manager.sessions["other"].serverConnection
	otherName := tools.DefinitionName(manager.sessions["other"].tools[0].definition)
	before := otherRequests.Load()
	fail.Store(true)
	if status := manager.RefreshServer(t.Context(), "target"); status.Status != "error" {
		t.Fatalf("target failure = %+v", status)
	}
	if manager.sessions["other"].serverConnection != original || otherRequests.Load() != before {
		t.Fatal("selective refresh replaced or contacted another server")
	}
	if count := len(registry.Definitions()); count != 1 {
		t.Fatalf("remaining tools = %d", count)
	}
	tool, ok := registry.Get(otherName)
	if !ok {
		t.Fatal("another server's tool namespace changed")
	}
	if _, err := tool.Execute(t.Context(), map[string]any{"value": "still available"}); err != nil {
		t.Fatalf("another server's live tool was broken: %v", err)
	}
}
