package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestAsyncRefreshStopsOnManagerClose(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "blocked"}, nil)
	remote.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, request mcpsdk.Request) (mcpsdk.Result, error) {
			if method == "tools/list" {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					close(cancelled)
					return nil, ctx.Err()
				}
			}
			return next(ctx, method, request)
		}
	})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil))
	defer upstream.Close()
	defer close(release)
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{ID: "blocked", Name: "blocked", URL: upstream.URL, Enabled: true}}}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.RefreshServersAsync("blocked")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("background refresh did not reach MCP discovery")
	}
	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel background discovery")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("MCP discovery did not receive cancellation")
	}
}

func TestCancelledQueuedRefreshPreservesLiveConnection(t *testing.T) {
	var blocked atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote"}, nil)
	remote.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{}, nil
	})
	remote.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			if method == "tools/list" && blocked.Load() {
				once.Do(func() { close(entered) })
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return next(ctx, method, req)
		}
	})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil))
	defer upstream.Close()
	store := &testStore{servers: []mcpconfig.Server{{ID: "remote", Name: "remote", URL: upstream.URL, Enabled: true}}}
	registry := tools.NewRegistry()
	manager := NewManager(store, nil, registry, log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(t.Context())
	original := manager.sessions["remote"].serverConnection
	blocked.Store(true)
	refreshDone := make(chan struct{})
	go func() {
		manager.Refresh(t.Context())
		close(refreshDone)
	}()
	defer func() {
		close(release)
		<-refreshDone
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not reach the remote catalog")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	retryDone := make(chan mcpconfig.ServerStatus, 1)
	go func() {
		retryDone <- manager.RefreshServer(ctx, "remote")
	}()
	select {
	case status := <-retryDone:
		if status.Status != "error" || status.Error != context.DeadlineExceeded.Error() {
			t.Fatalf("cancelled retry = %#v", status)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled retry remained blocked behind another refresh")
	}
	if manager.Status("remote").Status != "connected" || manager.session("remote").serverConnection != original {
		t.Fatal("cancelled retry replaced the live connection")
	}
	definitions := registry.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("live tools = %d, want 1", len(definitions))
	}
	tool, ok := registry.Get(tools.DefinitionName(definitions[0]))
	if !ok {
		t.Fatal("cancelled retry removed the live tool")
	}
	if _, err := tool.Execute(t.Context(), nil); err != nil {
		t.Fatalf("live tool failed during refresh: %v", err)
	}
}

func TestProxyRefreshPreservesSessionAndActiveCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote"}, nil)
	remote.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		close(entered)
		select {
		case <-release:
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "finished"}}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil))
	defer upstream.Close()
	store := &testStore{}
	manager := NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	handler := manager.Handler()
	subscribed := make(chan struct{})
	var once sync.Once
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			once.Do(func() { close(subscribed) })
		}
		handler.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	changed := make(chan struct{}, 10)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test"}, &mcpsdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcpsdk.ToolListChangedRequest) { changed <- struct{}{} },
	})
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case <-subscribed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertTools := func(count int) []*mcpsdk.Tool {
		t.Helper()
		list, err := session.ListTools(ctx, nil)
		if err != nil || len(list.Tools) != count {
			t.Fatalf("tools = %v, error = %v, want %d", list, err, count)
		}
		return list.Tools
	}
	assertTools(0)
	store.servers = []mcpconfig.Server{{ID: "remote", Name: "remote", URL: upstream.URL, Enabled: true}}
	manager.Refresh(ctx)
	select {
	case <-changed:
	case <-ctx.Done():
		t.Fatal("existing client received no tools/list_changed")
	}
	name := assertTools(1)[0].Name
	original := manager.sessions["remote"].serverConnection
	revision := manager.Revision()
	manager.Refresh(ctx)
	manager.RefreshLocal(ctx)
	if manager.sessions["remote"].serverConnection != original || manager.Revision() != revision {
		t.Fatal("unchanged refresh replaced a connection or advanced the catalog")
	}
	store.err = errors.New("configuration unavailable")
	manager.Refresh(ctx)
	store.err = nil
	assertTools(1)
	if manager.sessions["remote"].serverConnection != original || manager.Revision() != revision {
		t.Fatal("failed configuration read changed the live catalog")
	}
	result := make(chan error, 1)
	go func() {
		response, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name})
		if err == nil && (response.IsError || len(response.Content) != 1) {
			err = io.ErrUnexpectedEOF
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	store.servers[0].Headers = []mcpconfig.Header{{Name: "X-Revision", Value: "2"}}
	manager.Refresh(ctx)
	if manager.sessions["remote"].serverConnection == original {
		t.Fatal("changed configuration reused old connection")
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("refresh interrupted active call: %v", err)
	}
	remote.AddTool(&mcpsdk.Tool{Name: "echo", Description: "updated", InputSchema: map[string]any{"type": "object", "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "updated"}}}, nil
	})
	manager.Refresh(ctx)
	if updated := assertTools(1)[0]; updated.Description != "MCP tool from remote: updated" {
		t.Fatalf("stale definition: %+v", updated)
	}
	response, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: map[string]any{"value": "ok"}})
	if err != nil || response.IsError {
		t.Fatalf("updated callback failed: %v, %v", response, err)
	}
	store.servers = nil
	manager.Refresh(ctx)
	assertTools(0)
	if _, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name}); err == nil {
		t.Fatal("removed tool remained callable")
	}
}

func TestIdleThreadDiscoversAndCallsServerConnectedMidThread(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote"}, nil)
	remote.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "finished"}}}, nil
	})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, nil))
	defer upstream.Close()
	store := &testStore{}
	manager := NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.sessionTTL = 150 * time.Millisecond
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	changed := make(chan struct{}, 1)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test"}, &mcpsdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcpsdk.ToolListChangedRequest) { changed <- struct{}{} },
	})
	thread := &http.Client{Transport: headerTransport{headers: []mcpconfig.Header{{Name: mcpsession.HeaderName, Value: "thread"}}}}
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL, HTTPClient: thread, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if list, err := session.ListTools(ctx, nil); err != nil || len(list.Tools) != 0 {
		t.Fatalf("tools before connecting = %v, %v", list, err)
	}
	time.Sleep(4 * manager.sessionTTL)
	store.servers = []mcpconfig.Server{{ID: "remote", Name: "remote", URL: upstream.URL, Enabled: true}}
	if status := manager.RefreshServer(ctx, "remote"); status.Status != "connected" {
		t.Fatalf("connect status = %+v", status)
	}
	if listed, _ := manager.ListedRevision("thread"); listed == manager.Revision() {
		t.Fatal("thread counted as current before its client read the new catalog")
	}
	select {
	case <-changed:
	case <-ctx.Done():
		t.Fatal("idle thread received no tools/list_changed for the new server")
	}
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 1 {
		t.Fatalf("tools after connecting = %v, %v", list, err)
	}
	if listed, ok := manager.ListedRevision("thread"); !ok || listed != manager.Revision() {
		t.Fatal("thread not counted as current after reading the new catalog")
	}
	if _, ok := manager.ListedRevision("other thread"); ok {
		t.Fatal("thread that never read the catalog counted as a reader")
	}
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: list.Tools[0].Name})
	if err != nil || result.IsError {
		t.Fatalf("call new server's tool = %v, %v", result, err)
	}
}

func TestProxySessionLivesOnlyWhileItsNotificationStreamIsConnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	manager := NewManager(&testStore{}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.sessionTTL = 150 * time.Millisecond
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	send := func(ctx context.Context, method, session, body string) *http.Response {
		t.Helper()
		request, _ := http.NewRequestWithContext(ctx, method, proxy.URL, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			request.Header.Set("Mcp-Session-Id", session)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	session := send(ctx, http.MethodPost, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`).Header.Get("Mcp-Session-Id")
	send(ctx, http.MethodPost, session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	streamCtx, drop := context.WithCancel(ctx)
	if status := send(streamCtx, http.MethodGet, session, "").StatusCode; status != http.StatusOK {
		t.Fatalf("notification stream status = %d", status)
	}
	const ping = `{"jsonrpc":"2.0","id":2,"method":"ping"}`
	time.Sleep(4 * manager.sessionTTL)
	if status := send(ctx, http.MethodPost, session, ping).StatusCode; status != http.StatusOK {
		t.Fatalf("session with a connected stream expired: status %d", status)
	}
	drop()
	time.Sleep(4 * manager.sessionTTL)
	if status := send(ctx, http.MethodPost, session, ping).StatusCode; status != http.StatusNotFound {
		t.Fatalf("session outlived its dropped stream: status %d", status)
	}
}
