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

func desktopPeer(t *testing.T, backend *DesktopBackend, session string) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		backend.Connect(r.Context(), session, conn)
	}))
	t.Cleanup(server.Close)
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		backend.mu.Lock()
		connected := backend.pages[session] != nil
		backend.mu.Unlock()
		if connected {
			return peer
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("desktop did not connect")
	return nil
}

func TestDesktopSessionIsolationAndTabs(t *testing.T) {
	backend := NewDesktopBackend()
	peer := desktopPeer(t, backend, "thread-1")
	if _, err := backend.Call(context.Background(), ActionInput{Session: "thread-2", Action: ActionTabs}); err == nil {
		t.Fatal("another thread obtained the connected browser")
	}
	if _, err := backend.Call(context.Background(), ActionInput{Session: "thread-1", Action: ActionClaimTab, TabID: "foreign"}); err == nil {
		t.Fatal("claimed a foreign tab")
	}
	go func() {
		var request cdpMessage
		if peer.ReadJSON(&request) == nil {
			_ = peer.WriteJSON(cdpMessage{ID: request.ID, Result: json.RawMessage(`{"id":"17","url":"https://example.com","title":"Example"}`)})
		}
	}()
	out, err := backend.Call(context.Background(), ActionInput{Session: "thread-1", Action: ActionTabs})
	if err != nil {
		t.Fatal(err)
	}
	var tabs TabsOutput
	if err := json.Unmarshal(out.Data, &tabs); err != nil {
		t.Fatal(err)
	}
	if len(tabs.Tabs) != 1 || tabs.Tabs[0].ID != "17" || tabs.Tabs[0].Ownership != "current_session" {
		t.Fatalf("tabs=%#v", tabs)
	}
}

func TestDesktopCancellationClosesPendingTransport(t *testing.T) {
	backend := NewDesktopBackend()
	peer := desktopPeer(t, backend, "thread-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := backend.Call(ctx, ActionInput{Session: "thread-1", Action: ActionNavigate, URL: "https://example.com"})
		result <- err
	}()
	var request cdpMessage
	if err := peer.ReadJSON(&request); err != nil || request.Method != "Jaz.open" {
		t.Fatalf("first command=%#v err=%v", request, err)
	}
	cancel()
	if err := <-result; err == nil {
		t.Fatal("cancelled navigation succeeded")
	}
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	if err := peer.ReadJSON(&request); err == nil {
		t.Fatal("cancelled request left the browser connection open")
	}
}

func TestDesktopScreenshotTimeoutPreservesConnection(t *testing.T) {
	backend := NewDesktopBackend()
	peer := desktopPeer(t, backend, "thread-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := backend.Call(ctx, ActionInput{Session: "thread-1", Action: ActionScreenshot})
		result <- err
	}()
	var request cdpMessage
	if err := peer.ReadJSON(&request); err != nil || request.Method != "Page.captureScreenshot" {
		t.Fatalf("screenshot command=%#v err=%v", request, err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stalled screenshot error=%v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("stalled screenshot did not return before the script deadline")
	}
	if err := peer.WriteJSON(cdpMessage{ID: request.ID, Result: json.RawMessage(`{"data":"late"}`)}); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := backend.Call(ctx, ActionInput{Session: "thread-1", Action: ActionTabs})
		result <- err
	}()
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	if err := peer.ReadJSON(&request); err != nil || request.Method != "Jaz.tab" {
		t.Fatalf("screenshot timeout broke the next command: %#v err=%v", request, err)
	}
	if err := peer.WriteJSON(cdpMessage{ID: request.ID, Result: json.RawMessage(`{"id":"17"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("late screenshot response broke the next action: %v", err)
	}
}

func TestDesktopInvalidNavigationDoesNotSendCommands(t *testing.T) {
	backend := NewDesktopBackend()
	desktopPeer(t, backend, "thread-1")
	if _, err := backend.Call(context.Background(), ActionInput{Session: "thread-1", Action: ActionNavigate, URL: "file:///secret"}); err == nil {
		t.Fatal("file navigation accepted")
	}
	backend.mu.Lock()
	page := backend.pages["thread-1"]
	backend.mu.Unlock()
	page.conn.mu.Lock()
	defer page.conn.mu.Unlock()
	if page.conn.nextID != 0 {
		t.Fatal("invalid navigation reached the desktop")
	}
}

func TestDesktopRejectsOverlapWithoutInterruptingActiveAction(t *testing.T) {
	backend := NewDesktopBackend()
	peer := desktopPeer(t, backend, "thread-1")
	active := make(chan error, 1)
	go func() {
		_, err := backend.Call(context.Background(), ActionInput{Session: "thread-1", Action: ActionTabs})
		active <- err
	}()
	var request cdpMessage
	if err := peer.ReadJSON(&request); err != nil {
		t.Fatal(err)
	}
	overlap := make(chan error, 1)
	go func() {
		_, err := backend.Call(context.Background(), ActionInput{Session: "thread-1", Action: ActionState})
		overlap <- err
	}()
	select {
	case err := <-overlap:
		if err == nil || !strings.Contains(err.Error(), "Another browser action") {
			t.Fatalf("overlap error=%v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("overlapping action waited for the active browser action")
	}
	if err := peer.WriteJSON(cdpMessage{ID: request.ID, Result: json.RawMessage(`{"id":"17"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := <-active; err != nil {
		t.Fatalf("overlap interrupted the active action: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.Call(ctx, ActionInput{Session: "thread-1", Action: ActionStatus}); err != context.Canceled {
		t.Fatalf("cancelled action error=%v", err)
	}
}
