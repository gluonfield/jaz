package visualize_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
)

func TestCanvasDiscoveryAndPresentationInAnOrdinaryThread(t *testing.T) {
	layout := runtimefiles.New(t.TempDir())
	store, err := sqlitestore.New(layout.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Runtime: storage.RuntimeACP, RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	boards := canvas.NewService(layout)
	bus := sessionevents.New()
	events := bus.Subscribe(t.Context(), session.ID)
	tools := visualize.NewService(store, store, bus, boards)
	server := mcp.NewServer(&mcp.Implementation{Name: "canvas-test", Version: "1"}, nil)
	tools.AddTo(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(mcpsession.HeaderName, session.ID)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(endpoint.Close)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "canvas-test", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: endpoint.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	guide, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: visualize.GuideMCPToolName, Arguments: map[string]any{"modules": []string{"canvas", "diagram"}}})
	if err != nil || guide.IsError {
		t.Fatalf("canvas discovery = %+v, %v", guide, err)
	}
	file := filepath.Join(boards.Dir(session.ID), canvas.FileName)
	text := guide.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{file, `"pages":[`, "JSON Canvas 1.0", "show_canvas", "side-panel tab", "Diagram types"} {
		if !strings.Contains(text, want) {
			t.Fatalf("canvas guide missing %q", want)
		}
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discovery created a canvas: %v", err)
	}
	if err := os.MkdirAll(boards.Dir(session.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", `{"pages":[]}`} {
		if raw != "" {
			if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: visualize.ShowCanvasMCPToolName, Arguments: map[string]any{}})
		if err != nil || !result.IsError {
			t.Fatalf("missing/invalid canvas presentation = %+v, %v", result, err)
		}
	}
	raw := `{"pages":[{"id":"plan","name":"Plan","nodes":[{"id":"note","type":"text","text":"Agent-created board","x":0,"y":0,"width":240,"height":120}],"edges":[]}]}`
	if err := os.WriteFile(file, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: visualize.ShowCanvasMCPToolName, Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent != nil || len(result.Content) != 2 || result.Content[0].(*mcp.TextContent).Text != visualize.SavedMessage || result.Content[1].(*mcp.TextContent).Text != visualize.DisplayReminder {
		t.Fatalf("canvas presentation = %+v, %v", result, err)
	}
	select {
	case event := <-events:
		if event.Artifact == nil || event.Artifact.Kind != sessionevents.PresentationCanvas || event.Artifact.Artifact != nil || event.Artifact.View != nil || !event.Artifact.Presented {
			t.Fatalf("canvas event = %+v", event)
		}
		encoded, err := json.Marshal(event)
		if err != nil || strings.Contains(string(encoded), file) || strings.Contains(string(encoded), `"file"`) {
			t.Fatalf("canvas presentation leaked a path: %s, %v", encoded, err)
		}
		stored, err := store.LoadSessionEvents(session.ID)
		if err != nil || len(stored) != 1 || stored[0].Seq != event.Seq {
			t.Fatalf("persisted presentation = %+v, %v", stored, err)
		}
	default:
		t.Fatal("canvas presentation did not publish a live event")
	}
	current, err := store.LoadSession(session.ID)
	if err != nil || (current.RuntimeRef != nil && current.RuntimeRef.ArtifactSurface != "") {
		t.Fatalf("presentation changed the thread surface: %+v, %v", current, err)
	}
}
