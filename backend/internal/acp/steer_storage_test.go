package acp_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestClaudeSteersWhileAnotherBackendTriesTheSameRoot(t *testing.T) {
	for _, input := range []string{"send now with screenshot", "MCP thread input"} {
		t.Run(input, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJazAgent(t, testContext(t), store, root, acp.AgentClaude, map[string]string{
				"JAZ_FAKE_ACP_PROMPT_QUEUEING": "1",
			})
			jaz.server.Workspace = root
			thread, err := jaz.manager.Spawn(jaz.ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "steer-live"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: thread.SessionID, Message: "block until cancelled"}); err != nil {
				t.Fatal(err)
			}
			waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.SessionID)

			contender, openErr := sqlitestore.New(root)
			if openErr == nil {
				t.Cleanup(func() { _ = contender.Close() })
			}

			const followUp = "Use the new layout"
			withScreenshot := input == "send now with screenshot"
			if withScreenshot {
				handler := jaz.server.Handler()
				path := "/v1/sessions/" + thread.SessionID
				attachment := uploadChatTestScreenshot(t, handler, path, "screenshot.png")
				body := strings.NewReader(`{"message":"` + followUp + `","attachment_ids":["` + attachment.ID + `"]}`)
				request := httptest.NewRequestWithContext(jaz.ctx, http.MethodPost, path+"/messages:stream", body)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				stream := response.Body.String()
				if response.Code != http.StatusOK || !strings.Contains(stream, `"type":"accepted"`) || strings.Contains(stream, `"type":"error"`) {
					t.Fatalf("Send now = %d %s", response.Code, stream)
				}
			} else {
				tools := acp.NewMCPTools(jaz.manager)
				if _, _, err := tools.Send(jaz.ctx, callAs(""), acp.MCPSendInput{ThreadID: thread.SessionID, Prompt: followUp}); err != nil {
					t.Fatal(err)
				}
			}
			jaz.idle(thread.SessionID)
			if openErr == nil {
				t.Fatal("a second backend opened the live turn's root")
			}
			requests, err := os.ReadFile(filepath.Join(root, "requests.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			wire := string(requests)
			if strings.Count(wire, `"method":"session/prompt"`) != 2 || !strings.Contains(wire, followUp) || strings.Contains(wire, `"method":"session/cancel"`) {
				t.Fatalf("follow-up did not steer the existing Claude turn: %s", wire)
			}
			if withScreenshot && !strings.Contains(wire, `"mimeType":"image/png"`) {
				t.Fatal("Claude did not receive the screenshot")
			}
		})
	}
}
