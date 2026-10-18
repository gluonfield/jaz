package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/coordinator"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/visualize"
)

// A canvas thread's instructions name its board, so every way an agent session
// starts, including restoring one after a restart, must know the thread.
func TestCanvasThreadKeepsItsBoardAcrossManagerRestart(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	requestLog := filepath.Join(t.TempDir(), "requests.jsonl")
	root := t.TempDir()
	newManager := func() *acp.Manager {
		return acp.NewManager(store, acp.Config{
			Root:         root,
			Workspace:    t.TempDir(),
			SystemPrompt: coordinator.NewBuilder(root, "", t.TempDir(), func() bool { return false }),
			Agents: map[string]acp.AgentConfig{
				acp.AgentKimi: {
					Command: os.Args[0],
					Args:    []string{"-test.run=TestFakeACPAgentProcess"},
					Env: map[string]string{
						"JAZ_FAKE_ACP_AGENT":       "1",
						"JAZ_FAKE_ACP_LOAD":        "1",
						"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
					},
				},
			},
		}, log.New(io.Discard))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	manager := newManager()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentKimi, Slug: "canvas", ArtifactSurface: string(visualize.SurfaceCanvas)})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, "first")
	manager.Close()

	manager = newManager()
	t.Cleanup(manager.Close)
	sendAndWait(t, ctx, manager, spawned.SessionID, "after manager restart")

	board := filepath.Join(root, "canvases", spawned.SessionID, "canvas.json")
	restored := false
	for _, request := range readFakeACPRequests(t, requestLog) {
		if request.Method != "session/load" {
			continue
		}
		var params struct {
			Meta map[string]any `json:"_meta"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		prompt, _ := params.Meta["systemPrompt"].(string)
		if !strings.Contains(prompt, board) {
			t.Fatalf("restored canvas session does not name its board %s", board)
		}
		restored = true
	}
	if !restored {
		t.Fatal("the restarted manager never restored the canvas session")
	}
}
