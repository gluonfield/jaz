package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/tools"
)

type appQueryInput struct {
	Query string `json:"query"`
}

func TestManagerServesMCPAppEntrypoints(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "tasks",
		Version: "1.0.0",
		Icons:   []mcpsdk.Icon{{Source: "data:image/svg+xml;base64,PHN2Zy8+", MIMEType: "image/svg+xml"}},
	}, nil)
	declared := mcpsdk.Meta{"ui": map[string]any{"csp": map[string]any{
		"connectDomains":  []string{"https://api.tasks.example", "https://evil.example; script-src *"},
		"resourceDomains": []string{"https://*.cdn.example:*"},
	}}}
	for uri, meta := range map[string]mcpsdk.Meta{"ui://tasks/app": declared, "ui://tasks/tray": nil, "ui://tasks/viewer": nil} {
		remote.AddResource(&mcpsdk.Resource{URI: uri, Name: uri, MIMEType: AppMIMEType}, func(_ context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
			return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: req.Params.URI, MIMEType: AppMIMEType, Text: "<!DOCTYPE html><div id=" + req.Params.URI + "></div>", Meta: meta}}}, nil
		})
	}
	answer := func(ctx context.Context, req *mcpsdk.CallToolRequest, input appQueryInput) (*mcpsdk.CallToolResult, any, error) {
		if _, ok := req.Session.InitializeParams().Capabilities.Extensions["io.modelcontextprotocol/ui"]; !ok {
			return nil, nil, errors.New("Jaz did not declare MCP Apps support")
		}
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: req.Params.Name + ":" + input.Query}}}, nil, nil
	}
	entry := func(uri string, entrypoints ...map[string]any) mcpsdk.Meta {
		list := make([]any, len(entrypoints))
		for i, e := range entrypoints {
			list[i] = e
		}
		return mcpsdk.Meta{"ui": map[string]any{"resourceUri": uri}, "openai/ui": map[string]any{"entrypoints": list}}
	}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "library", Title: "Library", Meta: entry("ui://tasks/app", map[string]any{"type": "global"})}, answer)
	tray := entry("ui://tasks/tray", map[string]any{"type": "thread"})
	tray["ui"] = map[string]any{"resourceUri": "ui://tasks/tray", "visibility": []string{"model"}}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "tray", Annotations: &mcpsdk.ToolAnnotations{Title: "Tray"}, Meta: tray}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "viewer", Meta: entry("ui://tasks/viewer", map[string]any{"type": "file", "extensions": []string{".STL", "step"}}, map[string]any{"type": "sidebar"})}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "search", Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/app"}}}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "graphql", Meta: mcpsdk.Meta{"ui": map[string]any{"visibility": []string{"app"}}}}, answer)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "admin", Meta: mcpsdk.Meta{"ui": map[string]any{"visibility": []string{"model"}}}}, answer)
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return remote
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	store := &testStore{servers: []mcpconfig.Server{{
		ID:        "srv1",
		Name:      "Tasks",
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       httpServer.URL,
		Enabled:   true,
	}}}
	registry := tools.NewRegistry()
	manager := NewManager(store, nil, registry, log.New(io.Discard))
	defer manager.Close()
	early, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if points, err := manager.Entrypoints(early); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Entrypoints before the first refresh = %#v, %v; want it to wait", points, err)
	}
	manager.Refresh(context.Background())

	var agentTools []string
	for _, def := range registry.Definitions() {
		agentTools = append(agentTools, tools.DefinitionName(def))
	}
	if got := strings.Join(agentTools, ","); strings.Contains(got, "graphql") || !strings.Contains(got, "search") || !strings.Contains(got, "admin") {
		t.Fatalf("agent tools = %s, want search and admin without the app-only graphql", got)
	}

	icon := "data:image/svg+xml;base64,PHN2Zy8+"
	want := []Entrypoint{
		{ServerID: "srv1", Tool: "library", Type: "global", Title: "Library", Icon: icon},
		{ServerID: "srv1", Tool: "tray", Type: "thread", Title: "Tray", Icon: icon},
		{ServerID: "srv1", Tool: "viewer", Type: "file", Title: "viewer", Icon: icon, Extensions: []string{".stl"}},
	}
	points, err := manager.Entrypoints(context.Background())
	if err != nil || !reflect.DeepEqual(points, want) {
		t.Fatalf("entrypoints = %#v, %v\nwant %#v", points, err, want)
	}
	policy := func(resources, connect string) string {
		return `<!DOCTYPE html><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'` + resources + `; style-src 'unsafe-inline'` + resources +
			`; img-src data: blob:` + resources + `; media-src data: blob:` + resources + `; font-src data:` + resources +
			`; connect-src ` + connect + `; frame-src 'none'; base-uri 'none'; form-action 'none'">`
	}
	reaches := policy(" https://*.cdn.example:*", "https://api.tasks.example")
	for tool, doc := range map[string]string{
		"library": reaches + "<div id=ui://tasks/app></div>",
		"tray":    policy("", "'none'") + "<div id=ui://tasks/tray></div>",
		"search":  reaches + "<div id=ui://tasks/app></div>",
	} {
		if html, err := manager.ReadApp(context.Background(), "srv1", tool); err != nil || html != doc {
			t.Fatalf("ReadApp(%s) = %q, %v\nwant %q", tool, html, err, doc)
		}
	}
	if _, err := manager.ReadApp(context.Background(), "srv1", "admin"); err != ErrAppNotFound {
		t.Fatalf("ReadApp(admin) err = %v", err)
	}

	for _, tool := range []string{"graphql", "tray", "library"} {
		result, err := manager.CallAppTool(context.Background(), "srv1", tool, json.RawMessage(`{"query":"q"}`), nil)
		if err != nil || result.Content[0].(*mcpsdk.TextContent).Text != tool+":q" {
			t.Fatalf("app calling %s: %+v %v", tool, result, err)
		}
	}
	if _, err := manager.CallAppTool(context.Background(), "srv1", "admin", nil, nil); !errors.Is(err, ErrAppToolDenied) {
		t.Fatalf("app calling a model-only tool: err = %v", err)
	}

	// A tool the server adds after the catalog loaded, as a new deploy of its
	// app does, is found when the app first calls it.
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "rename"}, answer)
	if result, err := manager.CallAppTool(context.Background(), "srv1", "rename", json.RawMessage(`{"query":"q"}`), nil); err != nil || result.Content[0].(*mcpsdk.TextContent).Text != "rename:q" {
		t.Fatalf("app calling a tool added since the catalog loaded: %+v %v", result, err)
	}

	if _, err := manager.ReadApp(context.Background(), "missing", "library"); err != ErrAppNotFound {
		t.Fatalf("ReadApp(missing) err = %v", err)
	}
}

type recordedEvents struct {
	mu                  sync.Mutex
	appended, published []sessionevents.Event
	err                 error
}

func (r *recordedEvents) AppendSessionEvents(_ string, events ...sessionevents.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	for i := range events {
		events[i].Seq = int64(len(r.appended) + i + 1)
	}
	r.appended = append(r.appended, events...)
	return nil
}

func (r *recordedEvents) Publish(event sessionevents.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, event)
}

func TestProxyShowsMCPAppsInCallersThread(t *testing.T) {
	remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tasks", Version: "1.0.0"}, nil)
	create := func(_ context.Context, req *mcpsdk.CallToolRequest, input appQueryInput) (*mcpsdk.CallToolResult, map[string]string, error) {
		return &mcpsdk.CallToolResult{Meta: mcpsdk.Meta{"color": "#f2c94c"}, IsError: input.Query == "fail"}, map[string]string{"identifier": "AUG-" + input.Query}, nil
	}
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "show_issue", Title: "Issue", Meta: mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://tasks/issue"}}, Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true}}, create)
	mcpsdk.AddTool(remote, &mcpsdk.Tool{Name: "list_issues"}, create)
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return remote
	}, &mcpsdk.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()
	events := &recordedEvents{}
	manager := NewManager(&testStore{servers: []mcpconfig.Server{{
		ID: "srv1", Name: "Tasks", Transport: mcpconfig.TransportStreamableHTTP, URL: httpServer.URL, Enabled: true,
	}}}, nil, tools.NewRegistry(), log.New(io.Discard), WithSessionEvents(events, events))
	defer manager.Close()
	manager.Refresh(context.Background())
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()

	connect := func(headers ...mcpconfig.Header) *mcpsdk.ClientSession {
		t.Helper()
		session, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent", Version: "1.0.0"}, nil).Connect(context.Background(), &mcpsdk.StreamableClientTransport{
			Endpoint: proxy.URL, HTTPClient: &http.Client{Transport: headerTransport{headers: headers}}, MaxRetries: -1, DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	// call returns what Jaz told the agent about showing the result, after
	// the server's own content.
	call := func(session *mcpsdk.ClientSession, tool, query string) string {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "mcp_Tasks_" + tool, Arguments: map[string]any{"query": query}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != (query == "fail") || result.Meta["color"] != "#f2c94c" || fmt.Sprint(result.StructuredContent) != "map[identifier:AUG-"+query+"]" {
			t.Fatalf("proxy changed %s result: %+v", tool, result)
		}
		if len(result.Content) == 1 {
			return ""
		}
		return result.Content[1].(*mcpsdk.TextContent).Text
	}
	agent := connect(mcpconfig.Header{Name: mcpsession.HeaderName, Value: "thread-1"})
	catalog, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range catalog.Tools {
		if spec.Name == "mcp_Tasks_show_issue" && (readMeta[toolMeta](spec.Meta).UI.ResourceURI != "ui://tasks/issue" || spec.Title != "Issue" || spec.Annotations == nil || !spec.Annotations.ReadOnlyHint || spec.OutputSchema == nil) {
			t.Fatalf("proxy lost presentation metadata: %+v", spec)
		}
	}
	if told := call(agent, "show_issue", "12"); !strings.Contains(told, "Jaz has presented") {
		t.Fatalf("agent did not receive presentation feedback: %q", told)
	}
	if told := call(agent, "list_issues", "13") + call(agent, "show_issue", "fail"); told != "" {
		t.Fatalf("a tool without a view, or a failed call, claimed a display: %q", told)
	}
	if told := call(connect(), "show_issue", "14"); !strings.Contains(told, "has not displayed") {
		t.Fatalf("a call outside a thread claimed a display: %q", told)
	}
	manager.AppVisible = func(string) bool { return false }
	if told := call(agent, "show_issue", "16"); !strings.Contains(told, "has not displayed") {
		t.Fatalf("private turn claimed public delivery: %q", told)
	}
	manager.AppVisible = nil
	events.err = errors.New("disk full")
	if told := call(agent, "show_issue", "17"); !strings.Contains(told, "could not save") {
		t.Fatalf("display failure lost the successful tool result: %q", told)
	}

	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.appended) != 1 || len(events.published) != 1 || events.published[0].Seq != events.appended[0].Seq {
		t.Fatalf("only displayed calls open apps: appended %d, published %d", len(events.appended), len(events.published))
	}
	event := events.appended[0]
	var result mcpsdk.CallToolResult
	if err := json.Unmarshal(event.MCPApp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if event.SessionID != "thread-1" || event.Type != sessionevents.TypeMCPApp || event.MCPApp.ServerID != "srv1" || event.MCPApp.Tool != "show_issue" || !event.MCPApp.Presented ||
		string(event.MCPApp.Arguments) != `{"query":"12"}` || result.Meta["color"] != "#f2c94c" || fmt.Sprint(result.StructuredContent) != "map[identifier:AUG-12]" || len(result.Content) != 1 {
		t.Fatalf("event = %+v, result = %+v", event, result)
	}
}
