package acp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

func nativeSteeringFixture(t *testing.T, response string) (*jazProcess, string) {
	t.Helper()
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{
		"JAZ_FAKE_ACP_NATIVE_STEERING":       "1",
		"JAZ_FAKE_ACP_NATIVE_STEER_RESPONSE": response,
	})
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "native-steering", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread.ID, Message: "block until cancelled"}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ID)
	return jaz, thread.ID
}

func TestRejectedNativeSteersLeaveTheActiveTurnRunning(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		message  string
	}{
		{"failed", `{"outcome":"failed"}`, "adapter did not report a cause"},
		{"rpcError", "rpcError", "native request rejected"},
		{"unknown", `{"outcome":"unknown"}`, "invalid native ACP steering response"},
		{"incomplete", `{"outcome":"injected"}`, "invalid native ACP steering response"},
		{"unconfirmedIdle", `{"outcome":"promptRequired","reason":"unknown"}`, "invalid native ACP steering response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jaz, thread := nativeSteeringFixture(t, tc.response)
			for _, message := range []string{"reject followup first", "reject followup second"} {
				if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread, Message: message}); err != nil {
					t.Fatal(err)
				}
			}
			jaz.eventually("both rejected followups", func() bool {
				if job := jaz.status(thread); job.State != acp.StateRunning {
					t.Fatalf("rejection ended the original turn: %s %q", job.State, job.Error)
				}
				events, err := jaz.store.LoadSessionEvents(thread)
				if err != nil {
					t.Fatal(err)
				}
				n := 0
				for _, event := range events {
					if event.ACP != nil && strings.Contains(event.ACP.Error, tc.message) {
						n++
					}
				}
				return n >= 2
			})
			job := jaz.status(thread)
			if job.State != acp.StateRunning || jaz.session(thread).Turn == nil || len(job.ToolCalls) != 1 || job.ToolCalls[0].Status != "in_progress" {
				t.Fatalf("rejection ended the original turn: state=%s tools=%+v", job.State, job.ToolCalls)
			}
			if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread, Message: "accepted followup"}); err != nil {
				t.Fatal(err)
			}
			jaz.idle(thread)
			if job := jaz.status(thread); job.Error != "" {
				t.Fatalf("completion after rejection = %s %q", job.State, job.Error)
			}
			requests, err := os.ReadFile(filepath.Join(jaz.root, "requests.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(requests), `"method":"session/prompt"`) != 1 || strings.Contains(string(requests), `"method":"session/cancel"`) || strings.Count(string(requests), `"method":"_session/steering"`) != 3 {
				t.Fatalf("steering retried or cancelled: %s", requests)
			}
		})
	}
}

func TestNativeIdleRacePromptsEachFollowupOnceInOrder(t *testing.T) {
	jaz, thread := nativeSteeringFixture(t, `{"outcome":"failed"}`)
	for _, message := range []string{"idle followup one", "idle followup two"} {
		if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread, Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	jaz.idle(thread)
	prompts := jaz.prompts()
	if len(prompts) != 3 || !strings.Contains(prompts[1], "idle followup one") || !strings.Contains(prompts[2], "idle followup two") {
		t.Fatalf("idle race delivery = %q", prompts)
	}
	if session := jaz.session(thread); session.Error != "" || session.Turn != nil {
		t.Fatalf("idle fallback left an active/error turn: %+v", session.Turn)
	}
	messages, err := jaz.store.LoadMessages(thread)
	if err != nil {
		t.Fatal(err)
	}
	var userMessages []string
	for _, message := range messages {
		if provider.MessageRole(message) == "user" {
			userMessages = append(userMessages, provider.MessageContent(message))
		}
	}
	if strings.Join(userMessages, ",") != "block until cancelled,idle followup one,idle followup two" {
		t.Fatalf("fallback duplicated transcript input: %q", userMessages)
	}
}

func TestRejectedSteerPreservesTheNativeTurnFailure(t *testing.T) {
	jaz, thread := nativeSteeringFixture(t, `{"outcome":"failed"}`)
	if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread, Message: "reject followup after provider failure"}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("provider failure completion", func() bool { return jaz.session(thread).Turn == nil })
	if session := jaz.session(thread); session.Status != storage.StatusError || session.Error != "The requested model is not supported." {
		t.Fatalf("native provider failure replaced by steer rejection: %s %q", session.Status, session.Error)
	}
}
