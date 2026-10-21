package acp_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/httpapi/agentsessions"
	httpsessions "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/transcript"
	"github.com/wins/jaz/backend/internal/voice/live"
)

type voiceSessionTransport func(*http.Request) (*http.Response, error)

func (f voiceSessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestVoiceClarificationKeepsTheAgentTurnAndDeliversRepliesBeforeIdle(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Voice Fixture")
	capture := func(name string, data []byte) {
		t.Helper()
		if directory := os.Getenv("JAZ_VOICE_CAPTURE_DIR"); directory != "" {
			if err := os.WriteFile(filepath.Join(directory, name+".json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	service := live.NewService(store, func(string) (live.Credential, error) {
		return live.Credential{Token: "fixture"}, nil
	})
	if _, err := service.SaveSettings(live.Settings{Agent: "openai", Provider: live.ProviderOAuth}); err != nil {
		t.Fatal(err)
	}
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = voiceSessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "chatgpt.com" {
			t.Fatalf("unexpected voice provider destination: %s", r.URL.Host)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		capture("session-builder", data)
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("v=0\r\nanswer"))}, nil
	})
	if _, err := service.Connect(jaz.ctx, "v=0\r\noffer", "Current chat context: Voice Fixture", bot.ID); err != nil {
		t.Fatal(err)
	}
	input := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/agent/input", strings.NewReader(`{"message":"Who manufactures parts like Adrian? until released","contexts":[{"type":"voice","id":"call","request_id":"research","text":"User: Who manufactures parts like Adrian?"}]}`))
	response := httptest.NewRecorder()
	input.SetPathValue("session", bot.ID)
	agentsessions.NewHandler(jaz.manager).Input(response, input)
	if response.Code != http.StatusOK {
		t.Fatalf("voice input = %d: %s", response.Code, response.Body.String())
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	handler := httpsessions.NewMessagesHandler(transcript.NewService(store, jaz.manager))
	read := func(name string) map[string]json.RawMessage {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/messages?turns=32", nil)
		request.SetPathValue("session", bot.ID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("voice snapshot = %d: %s", response.Code, response.Body.String())
		}
		capture(name, response.Body.Bytes())
		var value map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	questionText := "Do you mean Hadrian, and other companies like it that manufacture parts?"
	if err := jaz.bots.Say(bot.ID, questionText); err != nil {
		t.Fatal(err)
	}
	events := jaz.events.Subscribe(jaz.ctx, bot.ID)
	answered := make(chan acp.AskUserOutput, 1)
	go func() {
		out, err := jaz.manager.AskUser(jaz.ctx, bot.ID, acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "manufacturer", Question: questionText}}})
		if err != nil {
			t.Error(err)
		}
		answered <- out
	}()
	var permission *sessionevents.ACPPermission
	for permission == nil {
		select {
		case event := <-events:
			if event.Type == "permission_request" {
				permission = event.Permission
			}
		case <-jaz.ctx.Done():
			t.Fatal(jaz.ctx.Err())
		}
	}
	waiting := read("waiting")
	if !strings.Contains(string(waiting["acp_permissions"]), permission.ID) || string(waiting["acp_state"]) != `"running"` {
		t.Fatalf("pending question missing from the real HTTP snapshot: %s", waiting["acp_permissions"])
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/interactive-response", strings.NewReader(`{"request_id":"`+permission.ID+`","answers":{"manufacturer":{"answers":["Yeah"]}}}`))
	jaz.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("spoken answer = %d: %s", response.Code, response.Body.String())
	}
	select {
	case out := <-answered:
		if out.Cancelled || len(out.Answers["manufacturer"].Answers) != 1 || out.Answers["manufacturer"].Answers[0] != "Yeah" {
			t.Fatalf("agent received %#v", out)
		}
	case <-jaz.ctx.Done():
		t.Fatal(jaz.ctx.Err())
	}
	if err := jaz.bots.Say(bot.ID, "I only searched memory and asked a clarification. I had not started a research thread."); err != nil {
		t.Fatal(err)
	}
	reply := read("reply-before-idle")
	if string(reply["acp_state"]) != `"running"` || !strings.Contains(string(reply["events"]), "I only searched memory") || !strings.Contains(string(reply["events"]), `"selected"`) {
		t.Fatalf("reply or answered question missing while agent runs: %s", reply["events"])
	}
	stored, err := store.LoadSession(bot.ID)
	if err != nil || len(stored.QueuedMessages) != 0 || jaz.manager.AwaitsAnswer(bot.ID) {
		t.Fatalf("answer remained waiting or queued: %#v, %v", stored, err)
	}
	jaz.release()
	jaz.idle(bot.ID)
	read("completed")
}
