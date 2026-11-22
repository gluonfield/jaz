package acp

import (
	"context"
	"encoding/json"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type testMCPRevision struct{ value atomic.Uint64 }

func (r *testMCPRevision) Revision() uint64 {
	return r.value.Load()
}

func TestMCPRefreshRequiresAdvertisedCapabilityAndPolicy(t *testing.T) {
	for _, test := range []struct{ name, raw, policy, want string }{
		{"native", `{"agentCapabilities":{"mcpCapabilities":{"http":true},"_meta":{"mcpRefresh":{"method":"_session/mcp/refresh"}}}}`, MCPServerPolicyAll, mcpRefreshMethod},
		{"widget", `{"agentCapabilities":{"mcpCapabilities":{"http":true},"_meta":{"mcpRefresh":{"method":"_session/mcp/refresh"}}}}`, MCPServerPolicyWidget, mcpRefreshMethod},
		{"restricted", `{"agentCapabilities":{"mcpCapabilities":{"http":true},"_meta":{"mcpRefresh":{"method":"_session/mcp/refresh"}}}}`, "memory_search", ""},
		{"grok", `{"agentCapabilities":{"mcpCapabilities":{"http":true}},"_meta":{"grokShell":true,"x.ai/mcp/sdk":true}}`, MCPServerPolicyAll, grokMCPUpdateMethod},
		{"grok without MCP SDK", `{"agentCapabilities":{"mcpCapabilities":{"http":true}},"_meta":{"grokShell":true}}`, MCPServerPolicyAll, ""},
		{"notifications", `{"agentCapabilities":{"mcpCapabilities":{"http":true}}}`, MCPServerPolicyAll, ""},
		{"unsupported transport", `{"agentCapabilities":{"_meta":{"mcpRefresh":{"method":"_session/mcp/refresh"}}}}`, MCPServerPolicyAll, ""},
		{"unknown method", `{"agentCapabilities":{"mcpCapabilities":{"http":true},"_meta":{"mcpRefresh":{"method":"_unknown"}}}}`, MCPServerPolicyAll, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := supportedMCPRefresh(json.RawMessage(test.raw), test.policy); got != test.want {
				t.Fatalf("refresh = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMCPRefreshBeforeTurnCoalescesAndRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "mcp-refresh", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{Agents: map[string]AgentConfig{"fake": {}}}, nil)
	revision := &testMCPRevision{}
	manager.SetMCPRevisionSource(revision)
	left, right := net.Pipe()
	clientConn, remoteConn := stdio.New(left, left), stdio.New(right, right)
	defer clientConn.Close()
	defer remoteConn.Close()
	requests := make(chan string, 10)
	var fail atomic.Bool
	remote := jsonrpc.NewPeer(remoteConn, jsonrpc.HandlerFunc(func(ctx context.Context, request jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		requests <- request.Method
		if request.Method == mcpRefreshMethod {
			var params struct {
				SessionID   string   `json:"sessionId"`
				ServerNames []string `json:"serverNames"`
			}
			if json.Unmarshal(request.Params, &params) != nil || params.SessionID != "native" || !reflect.DeepEqual(params.ServerNames, []string{"jaz_mcp"}) {
				return nil, jsonrpc.InvalidParams("wrong refresh scope", nil)
			}
			if fail.Load() {
				return nil, jsonrpc.InternalError("fixture failure", nil)
			}
			return json.RawMessage(`{}`), nil
		}
		if request.Method == "session/prompt" {
			return json.RawMessage(`{"stopReason":"cancelled"}`), nil
		}
		return nil, jsonrpc.MethodNotFound(request.Method)
	}))
	peer := jsonrpc.NewPeer(clientConn, jsonrpc.HandlerFunc(manager.handleJSONRPC))
	go remote.Serve(ctx)
	go peer.Serve(ctx)
	job := newIdleJob(session, "fake", "native", "", ModeState{})
	process := newAgentProcess(&agentConn{conn: clientConn, peer: peer})
	process.mcpRefresh = mcpRefreshMethod
	manager.addJob(job, process)
	expectRequest := func(want string) {
		t.Helper()
		select {
		case got := <-requests:
			if got != want {
				t.Fatalf("request = %q, want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	send := func() {
		t.Helper()
		if _, err := manager.Send(ctx, SendRequest{Session: session.ID, Message: "hello"}); err != nil {
			t.Fatal(err)
		}
		expectRequest("session/prompt")
		if done := job.turnDone(); done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	send()
	revision.value.Store(3)
	fail.Store(true)
	if _, err := manager.Send(ctx, SendRequest{Session: session.ID, Message: "retry me"}); err == nil {
		t.Fatal("failed refresh admitted a turn")
	}
	expectRequest(mcpRefreshMethod)
	messages, err := store.LoadMessages(session.ID)
	if err != nil || len(messages) != 1 {
		t.Fatalf("unaccepted message persisted: %d, %v", len(messages), err)
	}
	if process.mcpRevision != 0 {
		t.Fatal("failed refresh marked revision applied")
	}
	fail.Store(false)
	if _, err := manager.Send(ctx, SendRequest{Session: session.ID, Message: "hello again"}); err != nil {
		t.Fatal(err)
	}
	expectRequest(mcpRefreshMethod)
	expectRequest("session/prompt")
	if done := job.turnDone(); done != nil {
		<-done
	}
	send()
	if process.mcpRevision != 3 || manager.jobByID(session.ID) != job {
		t.Fatal("refresh lost revision or session identity")
	}
}

func TestGrokRefreshVersionsOnlyTheProxyConnection(t *testing.T) {
	manager := NewManager(nil, Config{MCPStore: staticMCPServerStore{servers: []mcpconfig.Server{
		{ID: "jaz_mcp", Name: "jaz_mcp", URL: "http://example.test/proxy", Transport: mcpconfig.TransportStreamableHTTP, Enabled: true, Headers: []mcpconfig.Header{{Name: mcpsession.HeaderName, Value: mcpsession.HeaderPlaceholder}}},
		{ID: "jaztools", Name: "jaztools", URL: "http://example.test/jaztools", Transport: mcpconfig.TransportStreamableHTTP, Enabled: true},
	}}}, nil)
	job := &jobState{Job: Job{ID: "jaz-session", ACPSession: "native"}}
	servers, err := manager.grokMCPServers(t.Context(), job, MCPServerPolicyWidget, 42)
	if err != nil || len(servers) != 2 {
		t.Fatalf("servers = %v, %v", servers, err)
	}
	var proxy, jaztools httpMCPServer
	if err := json.Unmarshal(servers[0], &proxy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(servers[1], &jaztools); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(proxy.Headers, []mcpconfig.Header{{Name: mcpsession.HeaderName, Value: "jaz-session"}, {Name: "X-Jaz-MCP-Revision", Value: "42"}}) {
		t.Fatalf("proxy headers = %+v", proxy.Headers)
	}
	if jaztools.URL != "http://example.test/jaztools?jaztools_surface=widget" || len(jaztools.Headers) != 0 {
		t.Fatalf("changed the direct tools configuration: %+v", jaztools)
	}
}

func TestGrokRefreshRequiresNativeAcknowledgement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	manager := NewManager(nil, Config{MCPStore: staticMCPServerStore{servers: []mcpconfig.Server{
		{ID: "jaz_mcp", Name: "jaz_mcp", URL: "http://example.test/proxy", Transport: mcpconfig.TransportStreamableHTTP, Enabled: true},
	}}}, nil)
	revision := &testMCPRevision{}
	revision.value.Store(1)
	manager.SetMCPRevisionSource(revision)
	left, right := net.Pipe()
	clientConn, remoteConn := stdio.New(left, left), stdio.New(right, right)
	defer clientConn.Close()
	defer remoteConn.Close()
	replies := make(chan json.RawMessage, 1)
	remote := jsonrpc.NewPeer(remoteConn, jsonrpc.HandlerFunc(func(ctx context.Context, request jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		var params struct {
			SessionID  string            `json:"sessionId"`
			MCPServers []json.RawMessage `json:"mcpServers"`
		}
		if request.Method != grokMCPUpdateMethod || json.Unmarshal(request.Params, &params) != nil || params.SessionID != "native" || params.MCPServers == nil {
			return nil, jsonrpc.InvalidParams("wrong update request", nil)
		}
		select {
		case reply := <-replies:
			return reply, nil
		case <-ctx.Done():
			return nil, jsonrpc.InternalError(ctx.Err().Error(), nil)
		}
	}))
	peer := jsonrpc.NewPeer(clientConn, jsonrpc.HandlerFunc(manager.handleJSONRPC))
	go remote.Serve(ctx)
	go peer.Serve(ctx)
	job := newIdleJob(storage.Session{ID: "refresh"}, "grok", "native", "", ModeState{})
	process := newAgentProcess(&agentConn{conn: clientConn, peer: peer})
	process.mcpPolicy = MCPServerPolicyAll
	process.mcpRefresh = grokMCPUpdateMethod
	manager.addJob(job, process)
	for _, reply := range []string{`{}`, `{"result":{"ok":false}}`, `{"result":{"ok":true}}`} {
		select {
		case replies <- json.RawMessage(reply):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		err := manager.refreshMCPBeforeTurn(ctx, job)
		if reply == `{"result":{"ok":true}}` {
			if err != nil || process.mcpRevision != 1 {
				t.Fatalf("successful update = %v, revision = %d", err, process.mcpRevision)
			}
		} else if err == nil || process.mcpRevision != 0 {
			t.Fatalf("unacknowledged update = %v, revision = %d", err, process.mcpRevision)
		}
	}
}
