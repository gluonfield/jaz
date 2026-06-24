package browserworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fallbackBackend struct {
	called bool
}

func (b *fallbackBackend) Call(context.Context, ActionInput) (ActionOutput, error) {
	b.called = true
	return ActionOutput{Status: "ok", Text: "fallback"}, nil
}

func TestExtensionBridgeRoutesCallToConnectedExtension(t *testing.T) {
	bridge := NewExtensionBridge(nil, nil)
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     ExtensionProtocol,
		"extension_id": "ext-1",
		"bridge_url":   "ws://127.0.0.1:5299/v1/browser/extension?key=secret",
		"user_agent":   "Chrome",
		"capabilities": map[string]any{"actions": SupportedExtensionActions()},
	}); err != nil {
		t.Fatal(err)
	}
	waitForConnected(t, bridge)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var req extensionCall
		if err := ws.ReadJSON(&req); err != nil {
			t.Errorf("read call: %v", err)
			return
		}
		if req.Type != "call" || req.Action != "snapshot" || req.Session != "browser-worker-1" {
			t.Errorf("request = %#v", req)
		}
		if err := ws.WriteJSON(extensionResult{
			ID:   req.ID,
			Type: "result",
			OK:   true,
			Output: extensionWireOutput{
				Status:        "ok",
				Text:          "snapshot text",
				ImageBase64:   "aW1hZ2U=",
				ImageMIMEType: "image/png",
			},
		}); err != nil {
			t.Errorf("write result: %v", err)
		}
	}()
	out, err := bridge.Call(context.Background(), ActionInput{Action: "snapshot", Session: "browser-worker-1"})
	if err != nil {
		t.Fatal(err)
	}
	<-done
	if out.Text != "snapshot text" || out.ImageBase64 != "aW1hZ2U=" || out.ImageMIMEType != "image/png" {
		t.Fatalf("out = %#v", out)
	}
	status := bridge.Status()
	if !status.Connected || status.ExtensionID != "ext-1" || status.Protocol != ExtensionProtocol || status.BridgeURL != "ws://127.0.0.1:5299/v1/browser/extension" || status.UserAgent != "Chrome" {
		t.Fatalf("status = %#v", status)
	}
}

func TestExtensionBridgeManagedModeBypassesConnectedExtension(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, func() bool { return false })
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     ExtensionProtocol,
		"extension_id": "ext-1",
		"capabilities": map[string]any{"actions": SupportedExtensionActions()},
	}); err != nil {
		t.Fatal(err)
	}
	waitForConnected(t, bridge)
	out, err := bridge.Call(context.Background(), ActionInput{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !fallback.called || out.Text != "fallback" {
		t.Fatalf("fallback=%v out=%#v", fallback.called, out)
	}
}

func TestExtensionBridgeAcceptsLegacyOptionalActions(t *testing.T) {
	bridge := NewExtensionBridge(nil, nil)
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	actions := requiredExtensionActions()
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     ExtensionProtocol,
		"extension_id": "old-ext",
		"capabilities": map[string]any{"actions": actions},
	}); err != nil {
		t.Fatal(err)
	}
	waitForConnected(t, bridge)
	status := bridge.Status()
	if !status.Connected || len(status.Actions) != len(actions) {
		t.Fatalf("status = %#v", status)
	}
	_, err = bridge.Call(context.Background(), ActionInput{Action: ActionExtract})
	if err == nil || !IsUnsupportedAction(err, ActionExtract) || !strings.Contains(err.Error(), "reload the updated Jaz Browser Bridge") {
		t.Fatalf("err = %v", err)
	}
}

func TestExtensionBridgeKeepsSocketInactiveUntilHello(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, func() bool { return false })
	bridge.Timeout = 10 * time.Millisecond
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	out, err := bridge.Call(context.Background(), ActionInput{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !fallback.called || out.Text != "fallback" {
		t.Fatalf("fallback=%v out=%#v", fallback.called, out)
	}
	if bridge.Status().Connected {
		t.Fatalf("extension socket activated before hello: %#v", bridge.Status())
	}
}

func TestExtensionBridgeRejectsInvalidHello(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, func() bool { return false })
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     "jaz.browser.extension.v0",
		"extension_id": "old-ext",
		"capabilities": map[string]any{"actions": SupportedExtensionActions()},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if reason := readCloseReason(t, ws); !strings.Contains(reason, "unsupported browser extension protocol") {
		t.Fatalf("close reason = %q", reason)
	}
	out, err := bridge.Call(context.Background(), ActionInput{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !fallback.called || out.Text != "fallback" {
		t.Fatalf("fallback=%v out=%#v", fallback.called, out)
	}
}

func TestExtensionBridgeRejectsMissingRequiredAction(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, func() bool { return false })
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     ExtensionProtocol,
		"extension_id": "broken-ext",
		"capabilities": map[string]any{"actions": []string{ActionStatus}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if reason := readCloseReason(t, ws); !strings.Contains(reason, "missing required capabilities") {
		t.Fatalf("close reason = %q", reason)
	}
	if bridge.Status().Connected {
		t.Fatalf("extension socket activated with missing required actions: %#v", bridge.Status())
	}
}

func TestExtensionBridgeClosesMalformedResult(t *testing.T) {
	bridge := NewExtensionBridge(nil, nil)
	server := httptest.NewServer(bridge)
	t.Cleanup(server.Close)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.WriteJSON(map[string]any{
		"type":         "hello",
		"protocol":     ExtensionProtocol,
		"extension_id": "ext-1",
		"capabilities": map[string]any{"actions": SupportedExtensionActions()},
	}); err != nil {
		t.Fatal(err)
	}
	waitForConnected(t, bridge)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"result","id":`)); err != nil {
		t.Fatal(err)
	}
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if reason := readCloseReason(t, ws); reason == "" {
		t.Fatal("missing close reason for malformed result")
	}
}

func TestExtensionBridgeFallsBackWhenDisconnected(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, func() bool { return false })
	out, err := bridge.Call(context.Background(), ActionInput{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !fallback.called || out.Text != "fallback" {
		t.Fatalf("fallback=%v out=%#v", fallback.called, out)
	}
}

func TestExtensionBridgeRequiresConnectedExtensionInExtensionMode(t *testing.T) {
	fallback := &fallbackBackend{}
	bridge := NewExtensionBridge(fallback, nil)
	_, err := bridge.Call(context.Background(), ActionInput{Action: "status"})
	if err == nil || !strings.Contains(err.Error(), "browser extension bridge is not connected") {
		t.Fatalf("err = %v", err)
	}
	if fallback.called {
		t.Fatal("extension mode should not use background Chromium fallback")
	}
}

func TestExtensionBridgeRejectsNonGet(t *testing.T) {
	bridge := NewExtensionBridge(nil, nil)
	req := httptest.NewRequest("POST", "/v1/browser/extension", strings.NewReader("{}"))
	res := httptest.NewRecorder()
	bridge.ServeHTTP(res, req)
	if res.Code != 404 {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestActionOutputMapsExtensionWireFields(t *testing.T) {
	raw := `{"status":"ok","text":"x","image_base64":"img","image_mime_type":"image/png","pdf_base64":"pdf","pdf_base64_length":3,"data":{"url":"https://example.com"}}`
	var wire extensionWireOutput
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		t.Fatal(err)
	}
	out := actionOutput(wire)
	if out.ImageBase64 != "img" || out.ImageMIMEType != "image/png" || out.PDFBase64 != "pdf" || out.PDFBase64Length != 3 || !strings.Contains(string(out.Data), "example.com") {
		t.Fatalf("out = %#v", out)
	}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/v1/browser/extension"
}

func waitForConnected(t *testing.T, bridge *ExtensionBridge) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if bridge.Status().Connected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("extension bridge did not accept hello")
}

func readCloseReason(t *testing.T, ws *websocket.Conn) string {
	t.Helper()
	_, _, err := ws.ReadMessage()
	if err == nil {
		t.Fatal("websocket stayed open")
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return closeErr.Text
	}
	return err.Error()
}
