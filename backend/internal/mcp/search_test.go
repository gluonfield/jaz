package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

type searchOutput struct {
	Results []map[string]any `json:"results"`
}

// searchServer serves a search tool that answers with results, or fails,
// beside a sidebar app tool and a plain app tool.
func searchServer(t *testing.T, annotations *mcpsdk.ToolAnnotations, calls *atomic.Int32, results []map[string]any, fail bool) string {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote", Version: "1.0.0", Icons: []mcpsdk.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+"}}}, nil)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "search", Annotations: annotations}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in appQueryInput) (*mcpsdk.CallToolResult, searchOutput, error) {
		calls.Add(1)
		if fail || in.Query != "acme" {
			return nil, searchOutput{}, errors.New("unavailable")
		}
		return nil, searchOutput{Results: results}, nil
	})
	open := func(context.Context, *mcpsdk.CallToolRequest, appQueryInput) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "show_tasks", Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/app"}, "openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "global"}}}}}, open)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "card", Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/card"}}}, open)
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	t.Cleanup(server.Close)
	return server.URL
}

func preview(tool string) map[string]any {
	return map[string]any{"openai/preview": map[string]any{"target": map[string]any{"type": "mcp_app_tool", "name": tool, "arguments": map[string]any{"view": "ENG-1"}}}}
}

func TestManagerSearchesConnectedServers(t *testing.T) {
	readOnly := &mcpsdk.ToolAnnotations{ReadOnlyHint: true}
	var tasksCalls, writerCalls, brokenCalls atomic.Int32
	issue := map[string]any{"id": "ENG-1", "title": "Fix login", "url": "https://tasks.example/issue/ENG-1", "text": "ENG-1 · Todo", "_meta": preview("show_tasks")}
	card := map[string]any{"id": "ENG-2", "title": "Ship card", "url": "https://tasks.example/issue/ENG-2", "_meta": preview("card"), "app": map[string]any{"tool": "show_tasks"}}
	servers := []mcpconfig.Server{
		{ID: "writer", Name: "Writer", URL: searchServer(t, nil, &writerCalls, []map[string]any{issue}, false)},
		{ID: "tasks", Name: "Tasks", URL: searchServer(t, readOnly, &tasksCalls, []map[string]any{
			issue,
			card,
			{"id": "x", "title": "Script", "url": "javascript:alert(1)"},
			{"id": "y", "title": "Local", "url": "file:///etc/passwd"},
			{"id": "z", "url": "https://tasks.example/issue/ENG-3"},
		}, false)},
		{ID: "broken", Name: "Broken", URL: searchServer(t, readOnly, &brokenCalls, nil, true)},
	}
	for i := range servers {
		servers[i].Transport = mcpconfig.TransportStreamableHTTP
		servers[i].Enabled = true
	}
	manager := NewManager(&testStore{servers: servers}, nil, tools.NewRegistry(), log.New(io.Discard))
	defer manager.Close()
	manager.Refresh(context.Background())

	sections, err := manager.Search(context.Background(), "acme")
	want := []SearchSection{{ServerID: "tasks", Name: "Tasks", Icon: "data:image/svg+xml;base64,PHN2Zy8+", Results: []SearchResult{
		{ID: "ENG-1", Title: "Fix login", URL: "https://tasks.example/issue/ENG-1", Text: "ENG-1 · Todo", App: &SearchApp{Tool: "show_tasks", Arguments: json.RawMessage(`{"view":"ENG-1"}`)}},
		{ID: "ENG-2", Title: "Ship card", URL: "https://tasks.example/issue/ENG-2"},
	}}}
	if err != nil || !reflect.DeepEqual(sections, want) {
		got, _ := json.Marshal(sections)
		t.Fatalf("sections = %s, %v", got, err)
	}
	if tasksCalls.Load() != 1 || brokenCalls.Load() != 1 || writerCalls.Load() != 0 {
		t.Fatalf("calls: tasks %d, broken %d, writer %d; a search tool that may write must never run", tasksCalls.Load(), brokenCalls.Load(), writerCalls.Load())
	}
}
