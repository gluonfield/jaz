package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCodexConsentReachesLaunchAndRetainedBotRefresh(t *testing.T) {
	for _, source := range []string{"", storage.SourceBot} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			store, err := sqlitestore.New(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			requestLog := filepath.Join(root, "requests.jsonl")
			capture := filepath.Join(root, "launch-prompt")
			newManager := func() *acp.Manager {
				return newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{
					"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog, "JAZ_FAKE_ACP_CODEX_PROMPT_CAPTURE": capture,
				}, "", "", func(cfg *acp.Config) {
					cfg.SystemPrompt = coordinator.NewBuilder(root, "", "", nil)
					cfg.TurnNote = bots.TurnNote(store)
					cfg.ResumePrompt = func(session storage.Session) (promptmodule.Modules, error) {
						if session.SourceType == storage.SourceBot {
							return bots.Prompt(store, session)
						}
						return nil, nil
					}
				})
			}
			first := newManager()
			t.Cleanup(first.Close)
			session, err := first.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "consent", SourceType: source})
			if err != nil {
				t.Fatal(err)
			}
			if source == storage.SourceBot {
				if err := store.SaveBot(storage.BotRecord{ThreadID: session.ID, Kind: bots.KindBot}); err != nil {
					t.Fatal(err)
				}
			}
			approval := "I approve committing, pushing and releasing this verified correction."
			if _, err := first.Send(t.Context(), acp.SendRequest{Session: session.ID, Message: approval, FromUser: true, Completion: acp.CompletionInline}); err != nil {
				t.Fatal(err)
			}
			if job, err := first.Wait(t.Context(), acp.WaitRequest{Session: session.ID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
				t.Fatalf("approved turn = %+v, %v", job, err)
			}
			launch, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			assertActionConsent(t, string(launch))
			first.Close()
			session, err = store.LoadSession(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			session.PromptDigest = "pre-consent-policy"
			if err := store.SaveSession(session); err != nil {
				t.Fatal(err)
			}
			second := newManager()
			t.Cleanup(second.Close)
			sendAndWait(t, t.Context(), second, session.ID, "Continue the approved work.")
			resumed := false
			for _, request := range readFakeACPRequests(t, requestLog) {
				if request.Method != "session/resume" {
					continue
				}
				resumed = true
				var params struct {
					Meta map[string]any `json:"_meta"`
				}
				if err := json.Unmarshal(request.Params, &params); err != nil {
					t.Fatal(err)
				}
				if _, set := params.Meta["systemPrompt"]; set {
					t.Fatal("Codex resume reattached persisted system instructions")
				}
			}
			prompts := agentPrompts(t, requestLog)
			if !resumed || len(prompts) != 2 {
				t.Fatalf("expected fresh and resumed turns, got resumed=%v prompts=%d", resumed, len(prompts))
			}
			if source == storage.SourceBot {
				assertActionConsent(t, prompts[1])
			} else if prompts[1] != "Continue the approved work." {
				t.Fatal("ordinary Codex resume injected an unsupported prompt refresh")
			}
			records, err := store.LoadMessageRecords(session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 2 || records[0].Content != approval || !records[0].FromUser || strings.Contains(records[1].Content, "## Irreversible actions") {
				t.Fatal("prompt refresh changed user authorization or leaked into the transcript")
			}
			t.Logf("source=%q launch_bytes=%d resumed=true system_prompt_reattached=false bot_policy_refreshed=%v user_authorization_preserved=true", source, len(launch), source == storage.SourceBot)
		})
	}
}

func assertActionConsent(t *testing.T, prompt string) {
	t.Helper()
	for _, rule := range []string{
		"Never take an irreversible action, including sending email or messages, without the user's explicit consent for that specific action.",
		"the exact message content, recipients and attachments. Wait for approval before acting.",
		"Research, analysis, drafting, access to data and general task instructions are not consent.",
		"Prior explicit authorization remains valid only within its approved scope; do not ask again while it still applies.",
		"Changes to the action, message content, recipients or attachments require fresh approval.",
	} {
		if strings.Count(prompt, rule) != 1 {
			t.Fatalf("provider prompt must carry consent rule once: %q", rule)
		}
	}
	if strings.Contains(prompt, "Ask first only before") || strings.Contains(prompt, "sending as the user") {
		t.Fatal("provider prompt retained the old consent exception")
	}
}
