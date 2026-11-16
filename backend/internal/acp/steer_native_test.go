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
		{"unsolicitedIdle", `{"outcome":"promptRequired","reason":"noRunningTurn"}`, "invalid native ACP steering response"},
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

func TestNativeIdleRaceKeepsOrderingAndActiveSteering(t *testing.T) {
	jaz, thread := nativeSteeringFixture(t, "nativeIdle")
	steers := []string{"idle followup one", "accepted followup two", "accepted followup third"}
	for _, message := range steers {
		if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: thread, Message: message}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(message, "third") {
			jaz.eventually("input accepted before turn completion", func() bool {
				job := jaz.status(thread)
				if job.State != acp.StateRunning {
					t.Fatalf("input ended the turn before its completion: %s %q", job.State, job.Error)
				}
				return strings.Contains(job.Assistant, message)
			})
		}
	}
	jaz.idle(thread)
	if job := jaz.status(thread); job.Error != "" {
		t.Fatalf("native idle delivery = %s %q", job.State, job.Error)
	}
	requests, err := os.ReadFile(filepath.Join(jaz.root, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(requests), `"method":"session/prompt"`) != 1 || strings.Count(string(requests), `"method":"_session/steering"`) != 3 || strings.Contains(string(requests), `"method":"session/cancel"`) {
		t.Fatalf("native input was replayed or cancelled: %s", requests)
	}
	session := jaz.session(thread)
	if session.Turn != nil || session.Usage.TotalTokens != 180 {
		t.Fatalf("native idle completion/usage = turn:%+v usage:%+v", session.Turn, session.Usage)
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
	events, err := jaz.store.LoadSessionEvents(thread)
	if err != nil {
		t.Fatal(err)
	}
	var assistant string
	for _, event := range events {
		if event.Type == "acp_message" {
			assistant += event.Content
		}
	}
	if assistant != strings.Join(steers, "\n")+"\n" {
		t.Fatalf("native consumed input order = %q", assistant)
	}
	if strings.Join(userMessages, ",") != "block until cancelled,"+strings.Join(steers, ",") {
		t.Fatalf("native idle steering duplicated transcript input: %q", userMessages)
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
