package mcp

import (
	"context"
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
	Results []SearchResult `json:"results"`
}

// searchServer serves a search tool that answers with results, or fails.
func searchServer(t *testing.T, annotations *mcpsdk.ToolAnnotations, calls *atomic.Int32, results []SearchResult, fail bool) string {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "remote", Version: "1.0.0", Icons: []mcpsdk.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+"}}}, nil)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "search", Annotations: annotations}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in appQueryInput) (*mcpsdk.CallToolResult, searchOutput, error) {
		calls.Add(1)
		if fail || in.Query != "acme" {
			return nil, searchOutput{}, errors.New("unavailable")
		}
		return nil, searchOutput{Results: results}, nil
	})
	server := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestManagerSearchesConnectedServers(t *testing.T) {
	readOnly := &mcpsdk.ToolAnnotations{ReadOnlyHint: true}
	var tasksCalls, writerCalls, brokenCalls atomic.Int32
	issue := SearchResult{ID: "ENG-1", Title: "Fix login", URL: "https://tasks.example/issue/ENG-1", Text: "ENG-1 · Todo"}
	servers := []mcpconfig.Server{
		{ID: "writer", Name: "Writer", URL: searchServer(t, nil, &writerCalls, []SearchResult{issue}, false)},
		{ID: "tasks", Name: "Tasks", URL: searchServer(t, readOnly, &tasksCalls, []SearchResult{
			issue,
			{ID: "x", Title: "Script", URL: "javascript:alert(1)"},
			{ID: "y", Title: "Local", URL: "file:///etc/passwd"},
			{ID: "z", URL: "https://tasks.example/issue/ENG-2"},
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
	want := []SearchSection{{ServerID: "tasks", Name: "Tasks", Icon: "data:image/svg+xml;base64,PHN2Zy8+", Results: []SearchResult{issue}}}
	if err != nil || !reflect.DeepEqual(sections, want) {
		t.Fatalf("sections = %#v, %v\nwant %#v", sections, err, want)
	}
	if tasksCalls.Load() != 1 || brokenCalls.Load() != 1 || writerCalls.Load() != 0 {
		t.Fatalf("calls: tasks %d, broken %d, writer %d; a search tool that may write must never run", tasksCalls.Load(), brokenCalls.Load(), writerCalls.Load())
	}
}
