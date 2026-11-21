package acp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotChatPendingSendIsNotAcknowledgedAcrossRestart(t *testing.T) {
	jaz := startFailingJaz(t, time.Hour)
	bot := jaz.newBot("Dispatcher")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "plan the trip provider fails 1 overloaded", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the bot waiting to retry", func() bool { return strings.Contains(jaz.status(bot.ID).Error, "Retrying in") })
	admitted := make(chan struct{})
	jaz.manager.ThreadStarted = func(storage.Session, string) { close(admitted) }
	response := httptest.NewRecorder()
	client, disconnect := context.WithCancel(jaz.ctx)
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/messages:stream", strings.NewReader(`{"message":"an unacknowledged follow-up"}`)).WithContext(client)
	finished := make(chan struct{})
	go func() {
		jaz.server.Handler().ServeHTTP(response, request)
		close(finished)
	}()
	select {
	case <-admitted:
	case <-finished:
		t.Fatalf("Send bypassed live admission: %s", response.Body.String())
	case <-jaz.ctx.Done():
		t.Fatal("Send did not reach the runtime")
	}
	disconnect()
	jaz.manager.Close()
	select {
	case <-finished:
	case <-jaz.ctx.Done():
		t.Fatal("Send did not fail when its backend stopped")
	}
	if strings.Contains(response.Body.String(), `"type":"accepted"`) {
		t.Fatalf("unreceived input was acknowledged: %s", response.Body.String())
	}
	jaz = jaz.restart(nil)
	jaz.idle(bot.ID)
	messages, err := jaz.store.LoadMessageRecords(bot.ID)
	if err != nil || len(messages) != 1 || len(jaz.session(bot.ID).QueuedMessages) != 0 || jaz.promptsContaining("an unacknowledged follow-up") != 0 {
		t.Fatalf("restart replayed unacknowledged input: messages=%+v, error=%v, prompts=%q", messages, err, jaz.prompts())
	}
}

func TestChatSendGeneratesItsTitleOnce(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	jaz.manager.ThreadStarted = jaz.server.TitleThread
	thread, err := jaz.manager.CreateSession(jaz.ctx, acp.SpawnRequest{Slug: "untitled", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+thread.ID+"/messages:stream", strings.NewReader(`{"message":"repair generated thread names"}`))
	jaz.server.Handler().ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"type":"accepted"`) {
		t.Fatalf("Send = %s", response.Body.String())
	}
	jaz.eventually("the generated title", func() bool { return jaz.session(thread.ID).TitleLocked })
	if jaz.promptsContaining("Write a concise 2-6 word display title") != 1 || jaz.promptsContaining("\"text\":\"repair generated thread names\"") != 1 {
		t.Fatalf("Send duplicated its first prompt or title: %q", jaz.prompts())
	}
}

func TestBotChatSendDuringProviderRetryReachesLiveInput(t *testing.T) {
	acp.SetProviderRetryDelays(t, time.Hour)
	store, root := newTestStore(t)
	env := failingProvider(root)
	env["JAZ_FAKE_ACP_NATIVE_STEERING"] = "1"
	env["JAZ_FAKE_ACP_UNIQUE_SESSION"] = "1"
	jaz := startJaz(t, testContext(t), store, root, env)
	bot := jaz.newBot("Dispatcher")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "plan the trip provider fails 1 overloaded", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the bot waiting to retry", func() bool { return strings.Contains(jaz.status(bot.ID).Error, "Retrying in") })
	admitted := make(chan string, 2)
	jaz.manager.ThreadStarted = func(session storage.Session, message string) {
		if session.ID == bot.ID {
			admitted <- message
		}
	}
	post := func(payload string) (*httptest.ResponseRecorder, <-chan struct{}) {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+bot.ID+"/messages:stream", strings.NewReader(payload))
		finished := make(chan struct{})
		go func() {
			jaz.server.Handler().ServeHTTP(response, request)
			close(finished)
		}()
		return response, finished
	}

	response, finished := post(`{"message":"also book the hotel","contexts":[{"type":"selection","text":"near the station"}]}`)
	select {
	case <-admitted:
	case <-finished:
		t.Fatalf("Send did not reach live admission: %s", response.Body.String())
	case <-jaz.ctx.Done():
		t.Fatal("Send did not reach the runtime")
	}
	second, secondFinished := post(`{"message":"then check the train"}`)
	select {
	case <-finished:
		t.Fatalf("Send finished before the bot could receive the message: %s", response.Body.String())
	case <-admitted:
		t.Fatal("the later message overtook pending input")
	case <-time.After(100 * time.Millisecond):
	}
	if queued := jaz.session(bot.ID).QueuedMessages; len(queued) != 0 {
		t.Fatalf("ordinary Send queued input: %+v", queued)
	}
	other := jaz.newBot("Other")
	otherResponse := httptest.NewRecorder()
	otherRequest := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+other.ID+"/messages:stream", strings.NewReader(`{"message":"check the weather"}`))
	jaz.server.Handler().ServeHTTP(otherResponse, otherRequest)
	if !strings.Contains(otherResponse.Body.String(), `"type":"accepted"`) {
		t.Fatalf("pending input blocked another bot: %s", otherResponse.Body.String())
	}
	if _, err := jaz.manager.Cancel(jaz.ctx, bot.ID); err != nil {
		t.Fatal(err)
	}
	for i, done := range []<-chan struct{}{finished, secondFinished} {
		select {
		case <-done:
		case <-jaz.ctx.Done():
			t.Fatal("Send did not continue after the turn ended")
		}
		if body := []*httptest.ResponseRecorder{response, second}[i].Body.String(); !strings.Contains(body, `"type":"accepted"`) || strings.Contains(body, `"type":"error"`) {
			t.Fatalf("Send = %s", body)
		}
	}
	jaz.idle(bot.ID)
	if jaz.promptsContaining("also book the hotel") != 1 || !strings.Contains(jaz.promptWith("also book the hotel"), "near the station") {
		t.Fatalf("input lost or duplicated: %q", jaz.prompts())
	}
	jaz = jaz.restart(nil)
	messages, err := jaz.store.LoadMessageRecords(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	var received []string
	for _, message := range messages {
		if message.Role == "user" && message.FromUser {
			received = append(received, message.Content)
		}
	}
	requests, err := os.ReadFile(filepath.Join(jaz.root, "requests.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(received, "|") != "plan the trip provider fails 1 overloaded|also book the hotel|then check the train" || len(jaz.session(bot.ID).QueuedMessages) != 0 || strings.Count(string(requests), "also book the hotel") != 1 || strings.Count(string(requests), "then check the train") != 1 {
		t.Fatalf("restart changed accepted input: messages=%+v, requests=%s", messages, requests)
	}
}
