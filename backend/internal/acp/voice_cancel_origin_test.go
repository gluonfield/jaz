package acp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memorydream"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestVoiceCancellationOriginSurvivesStorageAndClient(t *testing.T) {
	for _, origin := range []string{"user_stop", "unknown"} {
		t.Run(origin, func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, map[string]string{})
			bot := jaz.newBot("Stop Provenance")
			jaz.busy(bot.ID)
			_, answer := voiceMCPQuestion(t, jaz, bot.ID)
			mux := http.NewServeMux()
			mountVoiceTestRoutes(mux, jaz)
			mux.HandleFunc("/fixture/finish", func(w http.ResponseWriter, r *http.Request) {
				if origin == "user_stop" {
					request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/cancel", nil)
					response := httptest.NewRecorder()
					jaz.server.Handler().ServeHTTP(response, request)
					if response.Code != http.StatusOK {
						t.Errorf("Stop HTTP %d: %s", response.Code, response.Body.String())
					}
				} else if _, err := jaz.manager.Cancel(r.Context(), bot.ID); err != nil {
					t.Error(err)
				}
				jaz.eventually("the cancelled question's waiter leaving", func() bool {
					return !jaz.manager.AwaitsAnswer(bot.ID)
				})
				events, _ := store.LoadSessionEvents(bot.ID)
				var seq int64
				for _, event := range events {
					if event.ACP != nil && event.ACP.Origin == origin {
						seq = event.Seq
					}
				}
				if seq == 0 {
					t.Errorf("SQLite lost cancellation origin %s", origin)
				}
				// Slim status replacement and off-path compaction must preserve
				// the action that ended automatic clarification eligibility.
				if err := store.AppendSessionEvents(bot.ID,
					sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: bot.ID, State: "running"}},
					sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: bot.ID, State: "running"}},
					sessionevents.Event{Type: "acp_message", Content: "Later history.", ACP: &sessionevents.ACPEvent{ID: bot.ID, State: "running"}},
					sessionevents.Event{Type: "acp_message", Content: " Still later.", ACP: &sessionevents.ACPEvent{ID: bot.ID, State: "running"}},
					sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: bot.ID, State: "idle", StopReason: "end_turn"}},
				); err != nil {
					t.Error(err)
				}
				for range 10 {
					result, err := store.CompactNextSessionEvents(r.Context())
					if err != nil {
						t.Error(err)
						break
					}
					if result.ThreadID == "" || result.Complete {
						break
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]int64{"seq": seq})
			})
			mux.Handle("/", jaz.server.Handler())
			server := httptest.NewServer(mux)
			defer server.Close()
			runVoiceClient(t, server.URL, bot.ID, origin, "Yeah")
			if out := <-answer; !out.Cancelled || out.Answers == nil {
				t.Fatalf("ask_user = %+v", out)
			}
		})
	}
}

func TestVoiceNonUserCancellationOrigins(t *testing.T) {
	for _, caller := range []string{"worker cleanup", "dream timeout", "interactive fallback", "queue fallback"} {
		t.Run(caller, func(t *testing.T) {
			store, root := newTestStore(t)
			env := map[string]string{}
			if caller == "dream timeout" {
				env["JAZ_FAKE_ACP_BLOCK_PROMPT"] = "1"
				env["JAZ_FAKE_ACP_SET_CONFIG"] = "1"
			}
			jaz := startJaz(t, testContext(t), store, root, env)
			if caller == "worker cleanup" || caller == "dream timeout" {
				ctx, cancel := context.WithCancel(jaz.ctx)
				defer cancel()
				go func() {
					ticker := time.NewTicker(5 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							for _, job := range jaz.manager.List() {
								if len(job.ToolCalls) > 0 {
									cancel()
									return
								}
							}
						}
					}
				}()
				var err error
				if caller == "worker cleanup" {
					_, err = jaz.manager.RunWorker(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "cleanup", Directory: root}, "block until cancelled")
				} else {
					if _, err := settings.SaveMemorySettings(store, settings.MemorySettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
						t.Fatal(err)
					}
					_, err = memorydream.New(store, jaz.manager, sourcequeue.New(root)).RunDream(ctx, jazmem.DreamRequest{Root: root})
				}
				t.Logf("%s result: %v", caller, err)
				if err == nil {
					t.Fatal("expected automated cleanup after timeout/cancellation")
				}
			} else {
				bot := jaz.newBot("Fallback")
				jaz.busy(bot.ID)
				if caller == "interactive fallback" {
					if err := jaz.manager.AnswerInteractive(jaz.ctx, acp.InteractiveAnswer{Session: bot.ID, Text: "new input"}); err != nil {
						t.Fatal(err)
					}
				} else {
					session, _ := store.LoadSession(bot.ID)
					queued := storage.NewQueuedMessage("new input", nil)
					queued.ID = "queued-follow-up"
					session.QueuedMessages = []storage.QueuedMessage{queued}
					if err := store.SaveSession(session); err != nil {
						t.Fatal(err)
					}
					request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/queue", strings.NewReader(`{"op":"steer","id":"`+queued.ID+`"}`))
					response := httptest.NewRecorder()
					jaz.server.Handler().ServeHTTP(response, request)
					if response.Code != http.StatusOK {
						t.Fatalf("queue fallback = %d: %s", response.Code, response.Body.String())
					}
				}
				jaz.eventually("fallback input", func() bool {
					records, _ := store.LoadMessageRecords(bot.ID)
					return len(records) == 2
				})
				jaz.idle(bot.ID)
			}
			sessions, err := store.ListSessions(storage.SessionFilter{IncludeSourced: true})
			if err != nil {
				t.Fatal(err)
			}
			origins := 0
			for _, session := range sessions {
				events, err := store.LoadSessionEvents(session.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.ACP != nil && event.ACP.Origin != "" {
						origins++
						if event.ACP.Origin == "user_stop" {
							t.Fatalf("%s was falsely recorded as user abandonment", caller)
						}
					}
				}
			}
			if origins == 0 {
				t.Fatalf("%s did not exercise cancellation", caller)
			}
		})
	}
}

func TestVoiceInactiveCancellationOrigin(t *testing.T) {
	for _, user := range []bool{false, true} {
		t.Run(fmt.Sprint(user), func(t *testing.T) {
			store, root := newTestStore(t)
			jaz := startJaz(t, testContext(t), store, root, map[string]string{})
			bot := jaz.newBot("Inactive Question")
			jaz.write(bot.ID, "A completed turn")
			jaz.manager.Close()
			next := startJaz(t, jaz.ctx, store, root, map[string]string{})
			origin := "unknown"
			if user {
				origin = "user_stop"
				request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/cancel", nil)
				response := httptest.NewRecorder()
				next.server.Handler().ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("inactive Stop HTTP %d: %s", response.Code, response.Body.String())
				}
			} else if _, err := next.manager.Cancel(jaz.ctx, bot.ID); err != nil {
				t.Fatal(err)
			}
			events, err := store.LoadSessionEvents(bot.ID)
			if err != nil || events[len(events)-1].ACP.Origin != origin || len(next.manager.List()) != 0 {
				t.Fatalf("inactive cancellation must retain its actual origin without resuming: %+v, %v", events, err)
			}
		})
	}
}
