package mcpdeferred

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDeferredDiscoveryDispatchAndValidation(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ServerOptions{PageSize: 17})
	var calls atomic.Int32
	canceled := make(chan struct{})
	for i := range 80 {
		mcp.AddTool(server, &mcp.Tool{Name: fmt.Sprintf("tool_%02d", i), Description: fmt.Sprintf("Capability number %d", i), InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`)}, func(ctx context.Context, req *mcp.CallToolRequest, input struct {
			Value string `json:"value"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			calls.Add(1)
			if input.Value == "wait" {
				<-ctx.Done()
				close(canceled)
				return nil, nil, ctx.Err()
			}
			if req.Extra.Header.Get("X-Test-Session") != "same-session" {
				t.Error("dispatch lost session headers")
			}
			if req.Params.Meta["test"] != "preserved" {
				t.Error("dispatch lost tool metadata")
			}
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "original error"}}}, map[string]string{"native": "result"}, nil
		})
	}
	handler := Wrap(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Test-Session", "same-session")
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(host.Close)
	connect := func(path string) *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil)
		session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: host.URL + path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	direct := connect("")
	all, err := direct.ListTools(t.Context(), nil)
	if err != nil || len(all.Tools) != 17 || all.NextCursor == "" {
		t.Fatalf("direct catalogue: %v %v", all, err)
	}
	lazy := connect("?" + QueryParam + "=1")
	listed, err := lazy.ListTools(t.Context(), nil)
	if err != nil || len(listed.Tools) != 2 {
		t.Fatalf("deferred catalogue: %v %v", listed, err)
	}
	for _, offset := range []int{0, 75} {
		result, err := lazy.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_tools", Arguments: map[string]any{"offset": offset, "limit": 5}})
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Tools []mcp.Tool `json:"tools"`
			Total int        `json:"total_matches"`
			Next  int        `json:"next_offset"`
		}
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Tools) != 5 || page.Total != 80 || page.Tools[0].Name != fmt.Sprintf("tool_%02d", offset) || page.Tools[0].InputSchema == nil {
			t.Fatalf("lost catalogue capabilities: %s", data)
		}
	}
	matched, err := lazy.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_tools", Arguments: map[string]any{"query": "tool_79"}})
	if err != nil {
		t.Fatal(err)
	}
	var match struct {
		Tools []mcp.Tool `json:"tools"`
	}
	data, _ := json.Marshal(matched.StructuredContent)
	if json.Unmarshal(data, &match) != nil || len(match.Tools) != 1 || match.Tools[0].Name != "tool_79" {
		t.Fatalf("search returned wrong tool: %s", data)
	}
	params := &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_79", "arguments": map[string]any{"value": "ok"}}}
	params.Meta = map[string]any{"test": "preserved"}
	result, err := lazy.CallTool(t.Context(), params)
	if err != nil || !result.IsError || len(result.Content) != 1 || result.Content[0].(*mcp.TextContent).Text != "original error" || result.StructuredContent == nil || calls.Load() != 1 {
		t.Fatalf("dispatch changed result: %#v %v", result, err)
	}
	params.Arguments = map[string]any{"name": "tool_79", "arguments": map[string]any{"value": 42}}
	invalidResult, err := lazy.CallTool(t.Context(), params)
	if err == nil && !invalidResult.IsError {
		t.Fatal("original schema validation was bypassed")
	}
	if calls.Load() != 1 {
		t.Fatal("invalid arguments reached the tool")
	}
	for _, invalid := range []struct {
		name      string
		arguments any
	}{
		{"search_tools", json.RawMessage(`null`)},
		{"search_tools", []string{}},
		{"search_tools", map[string]any{"query": nil}},
		{"search_tools", map[string]any{"Query": "tool_79"}},
		{"search_tools", map[string]any{"limit": 0}},
		{"search_tools", map[string]any{"unexpected": true}},
		{"call_tool", map[string]any{"Name": "tool_79", "arguments": map[string]any{"value": "ok"}}},
		{"call_tool", map[string]any{"name": "tool_79", "arguments": map[string]any{"value": "ok"}, "unexpected": true}},
	} {
		if _, err := lazy.CallTool(t.Context(), &mcp.CallToolParams{Name: invalid.name, Arguments: invalid.arguments}); err == nil {
			t.Fatalf("accepted malformed %s arguments: %#v", invalid.name, invalid.arguments)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("malformed gateway arguments reached the original tool")
	}
	params.Arguments = map[string]any{"name": "not_available", "arguments": map[string]any{}}
	if result, err := lazy.CallTool(t.Context(), params); err == nil && !result.IsError {
		t.Fatal("unknown tool accepted")
	}
	params.Arguments = map[string]any{"name": "tool_79", "arguments": nil}
	if _, err := lazy.CallTool(t.Context(), params); err == nil {
		t.Fatal("null arguments accepted")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	params.Arguments = map[string]any{"name": "tool_79", "arguments": map[string]any{"value": "wait"}}
	if _, err := lazy.CallTool(ctx, params); err == nil {
		t.Fatal("cancellation was lost")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("underlying tool did not receive cancellation")
	}
}

func TestDeferredDoesNotEscapeRestrictedSurface(t *testing.T) {
	restricted := mcp.NewServer(&mcp.Implementation{Name: "restricted", Version: "1"}, nil)
	server := httptest.NewServer(Wrap(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return restricted }, &mcp.StreamableHTTPOptions{JSONResponse: true})))
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL + "?" + QueryParam + "=1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_79", "arguments": map[string]any{}}})
	if err == nil && !result.IsError {
		t.Fatal("restricted surface was bypassed")
	}
}

func TestDeferredTracksCatalogueChangesAndForwardsProgress(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "initial", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	host := httptest.NewServer(Wrap(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})))
	t.Cleanup(host.Close)
	progress := make(chan *mcp.ProgressNotificationParams, 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) { progress <- req.Params },
	})
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: host.URL + "?" + QueryParam + "=1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	search := func(want int) {
		t.Helper()
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_tools", Arguments: map[string]any{"query": "latest"}})
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Tools []mcp.Tool `json:"tools"`
		}
		data, _ := json.Marshal(result.StructuredContent)
		if json.Unmarshal(data, &page) != nil || len(page.Tools) != want {
			t.Fatalf("stale catalogue: %s", data)
		}
	}
	search(0)
	server.AddTool(&mcp.Tool{Name: "latest", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: req.Params.GetProgressToken(), Progress: 1, Total: 2, Message: "native progress"}); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "finished"}}}, nil
	})
	search(1)
	params := &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "latest", "arguments": map[string]any{}}}
	params.Meta = map[string]any{"progressToken": "same-operation"}
	result, err := session.CallTool(t.Context(), params)
	if err != nil || result.IsError {
		t.Fatalf("dynamic tool call failed: %v %v", result, err)
	}
	select {
	case event := <-progress:
		if event.ProgressToken != "same-operation" || event.Message != "native progress" || event.Progress != 1 {
			t.Fatalf("progress changed: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("tool progress did not reach the original client")
	}
	server.RemoveTools("latest")
	search(0)
	if result, err := session.CallTool(t.Context(), params); err == nil && !result.IsError {
		t.Fatal("removed tool is still callable")
	}
}
