package acp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// Codex keeps the instructions its thread started with, so turns after a
// bot's identity changes show it the new one until a turn succeeds, whether
// Jaz restarts before that turn or after it, and again after a compact
// rebuilds its context from the prompt its process was launched with. A
// thread starting its conversation starts with the current identity, and
// Claude, which renders the current prompt on every launch, is never shown it.
func TestABotWhoseAgentKeepsItsStartPromptIsShownAChangedIdentityUntilATurnSucceeds(t *testing.T) {
	for _, tc := range []struct {
		agent string
		shown []bool
	}{
		{acp.AgentCodex, []bool{false, false, true, true, false, false, false, true, false}},
		{acp.AgentClaude, []bool{false, false, false, false, false, false, false, false, false}},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			root := t.TempDir()
			requestLog := filepath.Join(root, "requests.jsonl")
			start := func() *acp.Manager {
				manager := newFakeNamedAgentManagerWithOptions(t, store, root, tc.agent, map[string]string{
					"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
					"JAZ_FAKE_ACP_EXPECT_PROMPT_TRIGGER": "refused", "JAZ_FAKE_ACP_EXPECT_PROMPT_CONTAINS": "never sent",
				}, "", "", func(cfg *acp.Config) { cfg.TurnNote = bots.TurnNote(store) })
				t.Cleanup(manager.Close)
				manager.TurnFinished = (&server.Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: sessionevents.New()}).HandleACPTurnFinished
				return manager
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			manager := start()
			bot, err := manager.CreateSession(ctx, acp.SpawnRequest{ACPAgent: tc.agent, Slug: "scout", Title: "Scout", SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			setPrompt := func(prompt string) {
				t.Helper()
				if err := store.SaveBot(storage.BotRecord{ThreadID: bot.ID, Kind: bots.KindBot, Prompt: prompt}); err != nil {
					t.Fatal(err)
				}
			}
			setPrompt("Codeword ALPHA.")
			messages := []string{"first", "second", "third, refused", "fourth", "fifth", "sixth", acp.CompactCommand, "seventh", "eighth"}
			sendAndWait(t, ctx, manager, bot.ID, messages[0])
			sendAndWait(t, ctx, manager, bot.ID, messages[1])
			setPrompt("Codeword BRAVO.")
			manager.Close()
			manager = start()
			if _, err := manager.Send(ctx, acp.SendRequest{Session: bot.ID, Message: messages[2], Completion: acp.CompletionInline}); err != nil {
				t.Fatal(err)
			}
			if job, err := manager.Wait(ctx, acp.WaitRequest{Session: bot.ID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateFailed {
				t.Fatalf("the refused turn = %#v, %v", job, err)
			}
			sendAndWait(t, ctx, manager, bot.ID, messages[3])
			sendAndWait(t, ctx, manager, bot.ID, messages[4])
			manager.Close()
			manager = start()
			sendAndWait(t, ctx, manager, bot.ID, messages[5])
			if _, err := manager.Compact(ctx, acp.CompactRequest{Session: bot.ID}); err != nil {
				t.Fatal(err)
			}
			if job, err := manager.Wait(ctx, acp.WaitRequest{Session: bot.ID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
				t.Fatalf("the compact = %#v, %v", job, err)
			}
			sendAndWait(t, ctx, manager, bot.ID, messages[7])
			sendAndWait(t, ctx, manager, bot.ID, messages[8])

			prompts := agentPrompts(t, requestLog)
			if len(prompts) != len(messages) {
				t.Fatalf("the agent got %d prompts, want %d: %q", len(prompts), len(messages), prompts)
			}
			for i, prompt := range prompts {
				identity := prompt != messages[i]
				if identity != tc.shown[i] || identity && !strings.Contains(prompt, "Codeword BRAVO.") || !strings.HasSuffix(prompt, messages[i]) {
					t.Fatalf("turn %q reached the agent as %q…; want the identity shown %v", messages[i], prompt[:min(len(prompt), 80)], tc.shown[i])
				}
			}
		})
	}
}

// agentPrompts is the text of every turn the fake agent was sent.
func agentPrompts(t *testing.T, requestLog string) []string {
	t.Helper()
	var prompts []string
	for _, request := range readFakeACPRequests(t, requestLog) {
		if request.Method != "session/prompt" {
			continue
		}
		var params struct {
			Prompt []struct {
				Text string `json:"text"`
			} `json:"prompt"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, params.Prompt[0].Text)
	}
	return prompts
}
