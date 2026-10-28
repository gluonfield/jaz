package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDesktopHostConnectsUnvisitedConversation(t *testing.T) {
	backend := NewDesktopBackend()
	defer backend.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			backend.ConnectHost(r.Context(), conn)
		}
	}))
	defer server.Close()
	host, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	deadline := time.Now().Add(time.Second)
	for {
		backend.mu.Lock()
		connected := backend.host != nil
		backend.mu.Unlock()
		if connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("desktop host did not connect")
		}
		time.Sleep(time.Millisecond)
	}
	for _, session := range []string{"unvisited", "cancelled"} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		result := make(chan error, 1)
		go func() {
			_, err := backend.Call(ctx, ActionInput{Session: session, Action: ActionStatus})
			result <- err
		}()
		var request cdpMessage
		_ = host.SetReadDeadline(time.Now().Add(time.Second))
		if err := host.ReadJSON(&request); err != nil {
			cancel()
			t.Fatal(err)
		}
		params, _ := json.Marshal(request.Params)
		if request.Method != "Jaz.connect" || string(params) != `{"session":"`+session+`"}` {
			cancel()
			t.Fatalf("unexpected connection request: %#v", request)
		}
		if err := host.WriteJSON(cdpMessage{ID: request.ID, Result: json.RawMessage(`{}`)}); err != nil {
			cancel()
			t.Fatal(err)
		}
		if session == "unvisited" {
			desktopPeer(t, backend, session)
			if err := <-result; err != nil {
				t.Fatal(err)
			}
		} else {
			cancel()
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled connection: %v", err)
			}
		}
		cancel()
	}
}
