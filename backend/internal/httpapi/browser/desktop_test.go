package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDesktopEndpointRequiresEnabledModeAndKnownSession(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := browsercontrol.NewDesktopBackend()
	defer backend.Close()
	handler := DesktopHandler{Backend: backend, Store: store}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/thread/browser", strings.NewReader(`{"action":"status"}`))
	request.SetPathValue("session", "thread")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disabled status=%d", response.Code)
	}
	if _, err := settings.SaveBrowserSettings(store, settings.BrowserSettings{Enabled: true, Mode: settings.BrowserModeDesktop}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown session status=%d", response.Code)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "browser-fixture", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if session.Slug == "" {
		t.Fatal("session has no slug")
	}
	for _, test := range []struct {
		body string
		want string
	}{
		{`{"action":"scroll","text":"dwon","amount":10}`, "unsupported scroll direction"},
		{`{"action":"scroll","text":"down","amount":-1}`, "amount must be non-negative"},
		{`{"action":"form_input","ref":"p1:e1","value":{}}`, "string, number or boolean"},
		{`{"action":"press"}`, "key is required"},
		{`{"action":"find"}`, "query is required"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
		request.SetPathValue("session", session.Slug)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.want) {
			t.Fatalf("input %s: status=%d body=%s", test.body, response.Code, response.Body)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/sessions/{session}/browser", handler)
	server := httptest.NewServer(mux)
	defer server.Close()
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/sessions/"+session.Slug+"/browser", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	deadline := time.Now().Add(time.Second)
	for {
		_, err = backend.Call(context.Background(), browsercontrol.ActionInput{Session: session.ID, Action: browsercontrol.ActionStatus})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("slug connection was not registered under canonical session ID: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"script"}`))
	request.SetPathValue("session", session.Slug)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("nested script status=%d", response.Code)
	}
}
