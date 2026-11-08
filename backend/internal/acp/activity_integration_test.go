package acp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

func TestBotActivityChunkSequenceReachesSafeHTTPAndInvalidatesLiveClient(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			prefix := filepath.Join(t.TempDir(), "gate")
			gates := []string{"thought", "response", "tool", "tool-end", "finish"}
			hold := func() {
				for _, gate := range gates {
					if err := os.WriteFile(prefix+"-"+gate, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			hold()
			errorFlag := "0"
			if fail {
				errorFlag = "1"
			}
			manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{"JAZ_FAKE_ACP_ACTIVITY_SEQUENCE": prefix, "JAZ_FAKE_ACP_ACTIVITY_ERROR": errorFlag})
			manager.Events = sessionevents.New()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			session, err := manager.CreateSession(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "phase-bot", SourceType: storage.SourceBot, Home: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Cancel(context.Background(), session.ID)
			changes := manager.Events.Subscribe(ctx, session.ID, sessionevents.TypeSession)
			handler := sessionsapi.NewMessagesHandler(transcript.NewService(store, manager))
			type response struct {
				Activity sessionevents.ACPActivity       `json:"acp_activity"`
				Tools    []sessionevents.ACPToolProgress `json:"acp_tool_progress"`
			}
			snapshot := func() response {
				req := httptest.NewRequest(http.MethodGet, "/messages", nil)
				req.SetPathValue("session", session.ID)
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, req)
				if out.Code != 200 {
					t.Fatalf("HTTP %d: %s", out.Code, out.Body.String())
				}
				var body response
				if err := json.Unmarshal(out.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out.Body.String(), "PRIVATE_") || strings.Contains(out.Body.String(), "SECRET_PASSWORD") {
					t.Fatal("private transcript leaked into bot messages")
				}
				return body
			}
			wait := func(phase string) response {
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					body := snapshot()
					if string(body.Activity.Phase) == phase {
						return body
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatalf("phase never became %s; final=%#v", phase, snapshot())
				return response{}
			}
			if _, err := manager.Send(ctx, acp.SendRequest{Session: session.ID, Message: "activity sequence", FromUser: true, Completion: acp.CompletionAsync}); err != nil {
				t.Fatal(err)
			}
			start := wait("starting")
			if start.Activity.TurnStartedAt.IsZero() {
				t.Fatal("turn boundary missing")
			}
			for {
				select {
				case <-changes:
					continue
				default:
				}
				break
			}
			for _, step := range []struct{ gate, phase string }{{"thought", "reasoning"}, {"response", "responding"}, {"tool", "tool"}, {"tool-end", "waiting"}} {
				if err := os.Remove(prefix + "-" + step.gate); err != nil {
					t.Fatal(err)
				}
				select {
				case <-changes:
				case <-time.After(5 * time.Second):
					t.Fatalf("phase %s did not invalidate live session query", step.phase)
				}
				got := wait(step.phase)
				if got.Activity.At.IsZero() || !got.Activity.TurnStartedAt.Equal(start.Activity.TurnStartedAt) {
					t.Fatalf("bad activity boundary: %#v", got.Activity)
				}
				if step.phase == "tool" && (len(got.Tools) != 1 || got.Tools[0].Program != "ssh") {
					t.Fatalf("safe tool metadata missing: %#v", got.Tools)
				}
			}
			os.Remove(prefix + "-finish")
			if fail {
				wait("error")
				return
			}
			wait("idle")
			hold()
			if _, err := manager.Send(ctx, acp.SendRequest{Session: session.ID, Message: "next turn", FromUser: true, Completion: acp.CompletionAsync}); err != nil {
				t.Fatal(err)
			}
			next := wait("starting")
			if !next.Activity.TurnStartedAt.After(start.Activity.TurnStartedAt) || len(next.Tools) != 0 {
				t.Fatalf("old turn leaked into new activity: %#v", next)
			}
			for _, gate := range gates {
				os.Remove(prefix + "-" + gate)
			}
			wait("idle")
		})
	}
}
