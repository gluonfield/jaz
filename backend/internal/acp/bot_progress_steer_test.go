package acp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

func TestUserSteeringSeesWorkersThatFinishedDuringTheBotsTurn(t *testing.T) {
	for _, method := range []string{"JAZ_FAKE_ACP_PROMPT_QUEUEING", "JAZ_FAKE_ACP_NATIVE_STEERING"} {
		t.Run(method, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, map[string]string{method: "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
			bot := jaz.newBot("Dispatcher")
			if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "Dispatch work: block until cancelled", FromUser: true}); err != nil {
				t.Fatal(err)
			}
			waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
			for i := range 20 {
				prompt := "Check stock"
				if i >= 16 {
					prompt += ": block until cancelled"
				}
				_, worker, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: fmt.Sprintf("stock-%02d", i), Prompt: prompt, Commitment: owe(t, store, bot.ID)})
				if err != nil {
					t.Fatal(err)
				}
				if i < 16 {
					jaz.idle(worker.ThreadID)
				} else {
					waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
				}
			}
			if strings.Contains(jaz.promptWith("Dispatch work"), "stock-") {
				t.Fatal("the opening note somehow contained future work")
			}
			if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: bot.ID, Message: "Any results yet?", FromUser: true}); err != nil {
				t.Fatal(err)
			}
			var prompt string
			jaz.eventually("the user steer reaching the agent", func() bool {
				requests, _ := os.ReadFile(filepath.Join(root, "requests.jsonl"))
				for _, line := range strings.Split(string(requests), "\n") {
					if strings.Contains(line, "Any results yet?") {
						prompt = line
						return true
					}
				}
				return false
			})
			for i := range 20 {
				state := "finished"
				if i >= 16 {
					state = "working"
				}
				if !strings.Contains(prompt, fmt.Sprintf("thread stock-%02d %s", i, state)) {
					t.Fatalf("steer omitted current worker progress: %s", prompt)
				}
			}
			t.Logf("steering request with 20 current commitments: %d bytes", len(prompt))
			messages, err := store.LoadMessageRecords(bot.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range messages {
				if message.Role == "user" && strings.Contains(message.Content, "open commitments") {
					t.Fatalf("the agent's progress note leaked into the user's message: %s", message.Content)
				}
			}
			jaz.release()
			jaz.idle(bot.ID)
		})
	}
}
