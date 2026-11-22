//go:build !windows

package acp_test

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestAntigravityThreadDiscoveryOnNaturalProcessStart(t *testing.T) {
	root := t.TempDir()
	store, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	requests := filepath.Join(root, "requests.jsonl")
	starts := filepath.Join(root, "starts")
	env := map[string]string{
		"JAZ_FAKE_ACP_MCP_HTTP":       "1",
		"JAZ_FAKE_ACP_EXPECT_MCP":     "1",
		"JAZ_FAKE_ACP_RESUME":         "1",
		"JAZ_FAKE_ACP_UNIQUE_SESSION": "1",
		"JAZ_FAKE_ACP_REQUEST_LOG":    requests,
		"JAZ_FAKE_ACP_START_LOG":      starts,
	}
	start := func() *acp.Manager {
		manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentAntigravity, env, "", "", func(config *acp.Config) {
			config.MCPStore = staticMCPStore{servers: []mcpconfig.Server{
				jaztools.ServerConfig("http://127.0.0.1:5299/mcp/jaztools"),
				mcpruntime.ProxyServerConfig("http://127.0.0.1:5299/mcp/proxy"),
			}}
		})
		manager.Events = sessionevents.New()
		manager.TurnFinished = (&server.Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: manager.Events}).HandleACPTurnFinished
		t.Cleanup(manager.Close)
		return manager
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	first := start()
	revision := &catalogRevision{}
	first.SetMCPRevisionSource(revision)
	thread, err := first.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentAntigravity, Slug: "scout"})
	if err != nil {
		t.Fatal(err)
	}
	native := thread.Session.RuntimeRef.SessionID
	for range 2 {
		revision.Add(1)
		if err := turn(ctx, first, thread.SessionID, "Inspect the existing work."); err != nil {
			t.Fatal(err)
		}
	}
	if processStarts(t, starts) != 1 {
		t.Fatal("catalogue changes retired the running process")
	}
	first.Close()
	second := start()
	if err := turn(ctx, second, thread.SessionID, "Continue the same work."); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.LoadSession(thread.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if native == "" || persisted.RuntimeRef.SessionID != native || persisted.RuntimeRef.DeferredMCPTools != nil {
		t.Fatalf("startup changed native identity or saved choice: %+v", persisted.RuntimeRef)
	}
	var attached []string
	for _, request := range readFakeACPRequests(t, requests) {
		if request.Method != "session/new" && request.Method != "session/resume" {
			continue
		}
		attached = append(attached, request.Method)
		var params struct {
			SessionID  string `json:"sessionId"`
			MCPServers []struct {
				Name    string             `json:"name"`
				URL     string             `json:"url"`
				Headers []mcpconfig.Header `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		if request.Method == "session/resume" && params.SessionID != native {
			t.Fatal("resume replaced native history")
		}
		if len(params.MCPServers) != 2 {
			t.Fatalf("%s changed connections: %+v", request.Method, params.MCPServers)
		}
		for _, server := range params.MCPServers {
			endpoint, err := url.Parse(server.URL)
			if err != nil || endpoint.Query().Get("jaz_deferred_tools") != "1" {
				t.Fatalf("%s left %s eager: %s", request.Method, server.Name, server.URL)
			}
			if len(server.Headers) != 1 || server.Headers[0].Name != mcpsession.HeaderName || server.Headers[0].Value != thread.SessionID {
				t.Fatalf("%s lost its session context: %+v", server.Name, server.Headers)
			}
		}
	}
	if len(attached) != 2 || attached[0] != "session/new" || attached[1] != "session/resume" || processStarts(t, starts) != 2 {
		t.Fatalf("unexpected process/session starts: %v", attached)
	}
}
