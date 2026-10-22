package acp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestThreadFollowUpUsesNativeSteeringWithoutStoppingWorker(t *testing.T) {
	for _, method := range []string{"JAZ_FAKE_ACP_NATIVE_STEERING", "JAZ_FAKE_ACP_PROMPT_QUEUEING", "unsupported"} {
		t.Run(method, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, map[string]string{method: "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
			bot := jaz.newBot("Designer")
			jaz.write(bot.ID, "Coordinate the design")
			tools := acp.NewMCPTools(jaz.manager)
			_, worker, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "design", Prompt: "Draft design: block until cancelled", Commitment: owe(t, store, bot.ID)})
			if err != nil {
				t.Fatal(err)
			}
			waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
			if method == "JAZ_FAKE_ACP_NATIVE_STEERING" {
				_, _, err := tools.Send(jaz.ctx, callAs(bot.ID), acp.MCPSendInput{ThreadID: worker.ThreadID, Prompt: "Switch to a plan", Plan: true})
				if err == nil || !strings.Contains(err.Error(), "cannot switch a running turn") {
					t.Fatalf("running mode switch = %v", err)
				}
			}
			before, err := store.LoadSession(worker.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = tools.Send(jaz.ctx, callAs(bot.ID), acp.MCPSendInput{ThreadID: worker.ThreadID, Prompt: "Use the latest hierarchy correction"})
			if method == "unsupported" {
				if err == nil || !strings.Contains(err.Error(), "steering unsupported") {
					t.Fatalf("unsupported steering = %v", err)
				}
				after, loadErr := store.LoadSession(worker.ThreadID)
				if loadErr != nil || after.Status != storage.StatusRunning || !after.Turn.StartedAt.Equal(before.Turn.StartedAt) {
					t.Fatalf("unsupported follow-up changed the running turn: %+v, %v", after, loadErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				jaz.eventually("the follow-up reaching the agent", func() bool {
					raw, _ := os.ReadFile(filepath.Join(root, "requests.jsonl"))
					return strings.Contains(string(raw), "latest hierarchy correction")
				})
				records, err := store.LoadMessageRecords(worker.ThreadID)
				var users []storage.Message
				for _, record := range records {
					if record.Role == "user" {
						users = append(users, record)
					}
				}
				if err != nil || len(users) != 2 || users[1].Content != "Use the latest hierarchy correction" || users[1].FromUser {
					t.Fatalf("worker follow-up transcript = %+v, %v", records, err)
				}
			}
			raw, err := os.ReadFile(filepath.Join(root, "requests.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"method":"session/cancel"`) {
				t.Fatalf("follow-up stopped the worker: %s", raw)
			}
			if method == "unsupported" {
				if _, err := jaz.manager.Cancel(jaz.ctx, worker.ThreadID); err != nil {
					t.Fatal(err)
				}
				return
			}
			jaz.idle(worker.ThreadID)
			jaz.eventually("the worker's result returning to its bot", func() bool { return jaz.promptsContaining("ACP session design") == 1 })
		})
	}
}

func TestIdleThreadFollowUpPreservesParentReplyRouting(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_NATIVE_STEERING": "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	bot, asker := jaz.newBot("Designer"), jaz.newBot("Asker")
	jaz.write(bot.ID, "Coordinate the design")
	if err := store.StartSessionTurn(bot.ID, storage.Turn{Output: &storage.TurnOutput{ReplyTo: asker.ID, Hops: 1}}); err != nil {
		t.Fatal(err)
	}
	worker, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "idle-design", ParentID: bot.ID, Directory: root, Commitment: owe(t, store, bot.ID)})
	if err != nil {
		t.Fatal(err)
	}
	tools := acp.NewMCPTools(jaz.manager)
	_, _, err = tools.Send(jaz.ctx, callAs(bot.ID), acp.MCPSendInput{ThreadID: worker.ID, Prompt: "Plan the design", Plan: true})
	if err != nil {
		t.Fatal(err)
	}
	jaz.idle(worker.ID)
	planned, err := jaz.manager.Status(worker.ID)
	if err != nil || planned.Modes.CurrentModeID != "plan" {
		t.Fatalf("idle follow-up lost plan mode: %+v, %v", planned.Modes, err)
	}
	_, _, err = tools.Send(jaz.ctx, callAs(bot.ID), acp.MCPSendInput{ThreadID: worker.ID, Prompt: "Revise the design until released"})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ID)
	saved, err := store.LoadSession(worker.ID)
	if err != nil || saved.Turn.ParentOutput == nil || saved.Turn.ParentOutput.ReplyTo != asker.ID {
		t.Fatalf("idle follow-up lost its requester: %+v, %v", saved.Turn, err)
	}
	jaz.release()
	jaz.idle(worker.ID)
	jaz.eventually("the requester routing reaching the parent's result queue", func() bool {
		for _, result := range jaz.queued(bot.ID) {
			if result.From == worker.ID && result.Output != nil && result.Output.ReplyTo == asker.ID {
				return true
			}
		}
		return false
	})
}
