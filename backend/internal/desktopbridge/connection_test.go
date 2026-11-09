package desktopbridge

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

func TestCancelledCallDoesNotWaitBehindOrWriteAfterAnotherWriter(t *testing.T) {
	ready := make(chan *Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connection := New(ws, 65536, nil)
		defer connection.Close()
		ready <- connection
		<-connection.Done()
	}))
	defer server.Close()
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	connection := <-ready
	defer connection.Close()
	connection.writes <- struct{}{}
	defer func() {
		select {
		case <-connection.writes:
		default:
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- connection.Call(ctx, "cancelled", nil, nil) }()
	deadline := time.Now().Add(time.Second)
	for {
		connection.mu.Lock()
		waiting := len(connection.pending) > 0
		connection.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("call never reached the occupied writer")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled writer error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled call waited for another writer")
	}
	<-connection.writes
	var value string
	go func() {
		result <- connection.Call(context.Background(), "next", map[string]string{"text": "kept"}, &value)
	}()
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	var request Message
	if err := peer.ReadJSON(&request); err != nil || request.Method != "next" {
		t.Fatalf("a cancelled request was written or damaged the next call: %#v %v", request, err)
	}
	if err := peer.WriteJSON(Message{ID: request.ID, Result: json.RawMessage(`"ok"`)}); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil || value != "ok" {
		t.Fatalf("connection could not be reused: %q %v", value, err)
	}
}
