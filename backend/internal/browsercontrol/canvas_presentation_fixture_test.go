//go:build browserintegration

package browsercontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/mcpsession"
	serverapi "github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionview"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
)

func mountCanvasPresentationFixture(t *testing.T, mux *http.ServeMux, store *sqlitestore.Store, boards *canvas.Service) {
	t.Helper()
	thread, err := store.CreateSession(storage.CreateSession{Slug: "canvas-agent", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex", Cwd: os.Getenv("JAZ_BROWSER_SMOKE_DIR")}})
	if err != nil {
		t.Fatal(err)
	}
	bus := sessionevents.New()
	tools := visualize.NewService(store, store, bus, boards)
	server := mcp.NewServer(&mcp.Implementation{Name: "jaztools", Version: "test"}, nil)
	tools.AddGuideTo(server)
	tools.AddCanvasTo(server)
	mux.Handle("/mcp/canvas", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	shell := (&serverapi.Server{Store: store, Events: bus}).Handler()
	mux.Handle("GET /v1/sessions/canvas-agent/events", shell)
	mux.HandleFunc("GET /v1/sessions/canvas-agent/messages", func(w http.ResponseWriter, _ *http.Request) {
		current, err := store.LoadSession(thread.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events, err := store.LoadSessionEvents(thread.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"session": sessionview.Public(current), "messages": []any{}, "events": sessionview.Events(events), "latest_event_seq": 0})
	})
	mux.Handle("POST /v1/sessions/canvas-agent/seen", shell)
	mux.HandleFunc("GET /v1/auth/check", func(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode(map[string]any{}) })
	mux.HandleFunc("GET /v1/sessions/canvas-agent/overview", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"agent_events": []any{}})
	})
	mux.HandleFunc("POST /exercise-canvas-present", func(w http.ResponseWriter, r *http.Request) {
		endpoint := "http://" + r.Host + "/mcp/canvas"
		if err := presentCanvasFixture(r.Context(), endpoint, thread.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		current, err := store.LoadSession(thread.ID)
		if err != nil || current.RuntimeRef.ArtifactSurface != "" {
			http.Error(w, "Canvas presentation changed the ordinary thread surface", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func presentCanvasFixture(ctx context.Context, endpoint, session string) error {
	directory := os.Getenv("JAZ_BROWSER_SMOKE_DIR")
	if binary := os.Getenv("JAZ_BROWSER_CODEX_BINARY"); binary != "" {
		config := fmt.Sprintf(`mcp_servers.jaztools={url=%q,http_headers={%q=%q},required=true,tools.artifact_guide.approval_mode="approve",tools.show_canvas.approval_mode="approve"}`, endpoint, mcpsession.HeaderName, session)
		command := exec.CommandContext(ctx, binary, "exec", "--ignore-user-config", "--ephemeral", "--json", "--skip-git-repo-check", "--sandbox", "workspace-write", "--model", "gpt-6.1-sol", "-C", directory, "-c", config, "-")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "OPENAI_API_KEY=") && !strings.HasPrefix(entry, "CODEX_API_KEY=") {
				command.Env = append(command.Env, entry)
			}
		}
		request := "Create a small editable canvas with one text card saying Agent-created canvas, and present it beside this conversation. Keep this as an ordinary conversation. Do not use the browser. Finish with JAZ_CANVAS_AGENT_OK when it is presented."
		command.Stdin = strings.NewReader(request)
		evidence, _ := json.MarshalIndent(map[string]any{"request": request, "model": "gpt-6.1-sol", "session_id": session, "artifact_surface": ""}, "", "  ")
		if err := os.WriteFile(filepath.Join(directory, "canvas-agent-request.json"), evidence, 0o600); err != nil {
			return err
		}
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if writeErr := os.WriteFile(filepath.Join(directory, "canvas-agent.jsonl"), output, 0o600); writeErr != nil {
			return writeErr
		}
		if err != nil {
			return fmt.Errorf("native canvas check: %w: %s", err, stderr.String())
		}
		if !bytes.Contains(output, []byte("JAZ_CANVAS_AGENT_OK")) || !bytes.Contains(output, []byte(visualize.ShowCanvasMCPToolName)) || !bytes.Contains(output, []byte(visualize.GuideMCPToolName)) {
			return fmt.Errorf("native agent did not finish canvas discovery and presentation; see canvas-agent.jsonl")
		}
		fmt.Println("CANVAS_PRESENTATION_REAL_AGENT_PASSED")
		return nil
	}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "canvas-fixture", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: &http.Client{Transport: sessionTransport{session: session}}}, nil)
	if err != nil {
		return err
	}
	defer client.Close()
	guide, err := client.CallTool(ctx, &mcp.CallToolParams{Name: visualize.GuideMCPToolName, Arguments: map[string]any{"modules": []string{"canvas"}}})
	if err != nil || guide.IsError {
		return fmt.Errorf("canvas guide: %+v, %v", guide, err)
	}
	text := guide.Content[0].(*mcp.TextContent).Text
	match := regexp.MustCompile("The canvas is `([^`]+)`").FindStringSubmatch(text)
	if len(match) != 2 {
		return fmt.Errorf("guide did not supply a canvas path")
	}
	if err := os.MkdirAll(filepath.Dir(match[1]), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(match[1], []byte(`{"pages":[{"id":"plan","name":"Plan","nodes":[{"id":"agent-note","type":"text","text":"Agent-created canvas","x":0,"y":0,"width":280,"height":120}],"edges":[]}]}`), 0o644); err != nil {
		return err
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: visualize.ShowCanvasMCPToolName, Arguments: map[string]any{}})
	if err != nil || result.IsError {
		return fmt.Errorf("canvas presentation: %+v, %v", result, err)
	}
	fmt.Println("CANVAS_PRESENTATION_SHARED_TOOL_CALLS_PASSED")
	return nil
}
