package acp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	httpsessions "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/transcript"
)

// The provider/media boundary is scripted. Session, HTTP, SQLite, SSE, MCP
// cancellation and ACP prompt handoff all run their production paths.
func TestVoiceAdmissionThroughSessionAndHTTP(t *testing.T) {
	for _, scenario := range []string{"teardown", "pending", "remount"} {
		for _, reply := range []string{"Yeah", "What is the time?", "stop"} {
			t.Run(scenario+"/"+reply, func(t *testing.T) {
				store, root := newTestStore(t)
				jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_NATIVE_STEERING": "1"})
				bot := jaz.newBot("Voice Admission")
				jaz.busy(bot.ID)
				question, answer := voiceMCPQuestion(t, jaz, bot.ID)
				var once sync.Once
				mux := http.NewServeMux()
				mountVoiceTestRoutes(mux, jaz)
				mux.HandleFunc("/fixture/finish", func(w http.ResponseWriter, r *http.Request) {
					once.Do(func() {
						jaz.release()
						jaz.idle(bot.ID)
					})
					events, err := store.LoadSessionEvents(bot.ID)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]int64{"seq": events[len(events)-1].Seq})
				})
				mux.Handle("/", jaz.server.Handler())
				server := httptest.NewServer(mux)
				defer server.Close()
				output := runVoiceClient(t, server.URL, bot.ID, scenario, reply)
				var receipt struct {
					Inputs []struct {
						Message  string                   `json:"message"`
						Contexts []storage.MessageContext `json:"contexts"`
					} `json:"inputs"`
				}
				if err := json.Unmarshal(output, &receipt); err != nil || len(receipt.Inputs) != 2 {
					t.Fatalf("client receipt = %s, %v", output, err)
				}
				out := <-answer
				if !out.Cancelled || out.Answers == nil || len(out.Answers) != 0 {
					t.Fatalf("real ask_user cancellation = %+v", out)
				}
				jaz.idle(bot.ID)
				records, err := store.LoadMessageRecords(bot.ID)
				if err != nil || len(records) != 2 || records[1].Content != reply || !records[1].FromUser {
					t.Fatalf("one backing input expected; records = %+v, %v", records, err)
				}
				if !strings.Contains(records[1].Blocks[0].Text, question.Questions[0].Question) {
					t.Fatalf("persisted input lost its question: %+v", records[1])
				}
				// A new manager has no renderer or in-memory admission set.
				jaz.manager.Close()
				next := startJaz(t, jaz.ctx, store, root, map[string]string{})
				body, _ := json.Marshal(receipt.Inputs[0])
				request := httptest.NewRequest(http.MethodPost, "/input", strings.NewReader(string(body)))
				request.SetPathValue("session", bot.ID)
				response := httptest.NewRecorder()
				agentsessions.NewHandler(next.manager).Input(response, request)
				if response.Code != http.StatusOK || len(next.manager.List()) != 0 {
					t.Fatalf("persisted retry restarted a provider: HTTP %d, jobs %+v", response.Code, next.manager.List())
				}
				messages, _ := store.LoadMessageRecords(bot.ID)
				if len(messages) != len(records) {
					t.Fatalf("persisted idempotence added a second input: %d -> %d", len(records), len(messages))
				}
			})
		}
	}
}

func voiceMCPQuestion(t *testing.T, jaz *jazProcess, session string) (*sessionevents.ACPPermission, <-chan acp.AskUserOutput) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "jaztools", Version: "1"}, nil)
	acp.NewMCPTools(jaz.manager).AddTo(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(mcpsession.HeaderName, session)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "scripted-agent", Version: "1"}, nil).Connect(jaz.ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	callctx, cancel := context.WithCancel(jaz.ctx)
	var questionID string
	t.Cleanup(func() {
		_ = jaz.manager.AnswerInteractive(context.Background(), acp.InteractiveAnswer{Session: session, RequestID: questionID, Answers: map[string]acp.InteractiveAnswerValue{}})
		cancel()
		_ = client.Close()
	})
	events := jaz.events.Subscribe(jaz.ctx, session)
	done := make(chan acp.AskUserOutput, 1)
	go func() {
		result, err := client.CallTool(callctx, &mcp.CallToolParams{Name: "ask_user", Arguments: acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "company", Question: "Do you mean Hadrian?"}}}})
		if err != nil {
			t.Error(err)
			done <- acp.AskUserOutput{}
			return
		}
		data, err := json.Marshal(result.StructuredContent)
		var out acp.AskUserOutput
		if err != nil || json.Unmarshal(data, &out) != nil {
			t.Errorf("ask_user schema: %s, %v", data, err)
		}
		done <- out
	}()
	for {
		select {
		case event := <-events:
			if event.Type == "permission_request" {
				questionID = event.Permission.ID
				return event.Permission, done
			}
		case <-jaz.ctx.Done():
			t.Fatal(jaz.ctx.Err())
		}
	}
}

func mountVoiceTestRoutes(mux *http.ServeMux, jaz *jazProcess) {
	mux.Handle("GET /v1/sessions/{session}/messages", httpsessions.NewMessagesHandler(transcript.NewService(jaz.store, jaz.manager)))
	mux.HandleFunc("POST /v1/sessions/{session}/agent/input", agentsessions.NewHandler(jaz.manager).Input)
}

func TestVoiceInputConcurrentRetriesAndNewerTurns(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_NATIVE_STEERING": "1"})
	bot := jaz.newBot("Voice Retries")
	for _, key := range []struct{ call, turn string }{{"call", "turn"}, {"call", "new-turn"}, {"other-call", "turn"}} {
		var group sync.WaitGroup
		start := make(chan struct{})
		for range 8 {
			group.Go(func() {
				<-start
				body := fmt.Sprintf(`{"message":"Yeah","contexts":[{"type":"voice","id":%q,"request_id":%q,"text":"User: Yeah"}]}`, key.call, key.turn)
				request := httptest.NewRequest(http.MethodPost, "/input", strings.NewReader(body))
				request.SetPathValue("session", bot.ID)
				response := httptest.NewRecorder()
				agentsessions.NewHandler(jaz.manager).Input(response, request)
				if response.Code != http.StatusOK {
					t.Errorf("concurrent input = %d: %s", response.Code, response.Body.String())
				}
			})
		}
		close(start)
		group.Wait()
		jaz.idle(bot.ID)
	}
	records, err := store.LoadMessageRecords(bot.ID)
	if err != nil || len(records) != 3 {
		t.Fatalf("one backing input per distinct call + turn expected: %+v, %v", records, err)
	}
}

func TestVoiceAnswerRacingOrdinaryInput(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_NATIVE_STEERING": "1"})
	bot := jaz.newBot("Answer Race")
	jaz.busy(bot.ID)
	permission, answer := voiceMCPQuestion(t, jaz, bot.ID)
	start := make(chan struct{})
	answered := make(chan error, 1)
	input := make(chan error, 1)
	go func() {
		<-start
		answered <- jaz.manager.AnswerInteractive(jaz.ctx, acp.InteractiveAnswer{Session: bot.ID, RequestID: permission.ID, Answers: map[string]acp.InteractiveAnswerValue{"company": {Answers: []string{"Yes"}}}})
	}()
	go func() {
		<-start
		_, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: bot.ID, Message: "stop", FromUser: true, Contexts: []storage.MessageContext{{Type: "voice", ID: "race-call", RequestID: "race-turn", Text: "Question: Do you mean Hadrian?\nUser: stop"}}})
		input <- err
	}()
	close(start)
	answerErr := <-answered
	if err := <-input; err != nil {
		t.Fatal(err)
	}
	out := <-answer
	if (answerErr == nil) == out.Cancelled {
		t.Fatalf("question resolution winner disagrees: answer error %v, ask_user %+v", answerErr, out)
	}
	jaz.release()
	jaz.idle(bot.ID)
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	resolved := 0
	for _, event := range events {
		if event.Type == "permission_response" && event.Permission.ID == permission.ID {
			resolved++
		}
	}
	records, err := store.LoadMessageRecords(bot.ID)
	if err != nil || resolved != 1 || len(records) != 2 || records[1].Content != "stop" {
		t.Fatalf("race must resolve once and retain raw stop: resolutions %d, records %+v, %v", resolved, records, err)
	}
}

func TestVoiceRetryPreservesUnconfirmedDispatch(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Unconfirmed Voice Input")
	contexts := []storage.MessageContext{{Type: "voice", ID: "call", RequestID: "native-turn", Text: "Question: Which company?\nUser: Yeah"}}
	if err := store.StartSessionTurn(bot.ID, storage.Turn{StartedAt: time.Now().UTC(), Prompt: &storage.TurnPrompt{Message: "Yeah", State: storage.PromptSending}}, storage.UserMessageRecord("Yeah", contexts, nil)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(bot.ID, storage.StatusInterrupted, storage.ErrPromptUnconfirmed.Error(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"message": "Yeah", "contexts": contexts})
	request := httptest.NewRequest(http.MethodPost, "/input", strings.NewReader(string(body)))
	request.SetPathValue("session", bot.ID)
	response := httptest.NewRecorder()
	agentsessions.NewHandler(jaz.manager).Input(response, request)
	session, err := store.LoadSession(bot.ID)
	records, recordErr := store.LoadMessageRecords(bot.ID)
	if response.Code != http.StatusOK || err != nil || recordErr != nil || len(jaz.manager.List()) != 0 || len(records) != 1 || session.Status != storage.StatusInterrupted || session.Error != storage.ErrPromptUnconfirmed.Error() || session.Turn.Prompt.State != storage.PromptSending {
		t.Fatalf("retry must acknowledge persistence and leave dispatch uncertain: HTTP %d, session %+v, records %+v, errors %v/%v", response.Code, session, records, err, recordErr)
	}
}

func runVoiceClient(t *testing.T, origin, session, scenario, reply string) []byte {
	t.Helper()
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Fatal("production voice-client tests require Bun 1.3.5 and frontend dependencies: install Bun, then run bun install --frozen-lockfile --ignore-scripts in frontend")
	}
	script, err := filepath.Abs("../../../frontend/src/renderer/src/lib/voice/testdata/admission-client.mjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, bun, script)
	command.Env = append(os.Environ(), "JAZ_VOICE_TEST_ORIGIN="+origin, "JAZ_VOICE_TEST_SESSION="+session, "JAZ_VOICE_TEST_SCENARIO="+scenario, "JAZ_VOICE_TEST_REPLY="+reply)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("production voice session: %v\n%s", err, output)
	}
	return output
}
