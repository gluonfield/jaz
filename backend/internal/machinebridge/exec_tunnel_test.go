package machinebridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type tunnelTestHubConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	pendMu  sync.Mutex
	pending map[string]chan Frame
	nextID  atomic.Int64

	frames chan ExecTunnelFrameParams
	closes chan ExecTunnelCloseParams
	done   chan struct{}
}

func startTunnelTestHub(t *testing.T, serverCaps Capabilities) (*httptest.Server, <-chan *tunnelTestHubConn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	conns := make(chan *tunnelTestHubConn, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != connectPath {
			http.NotFound(w, r)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ws.SetReadLimit(defaultReadLimit)
		var helloFrame Frame
		if err := ws.ReadJSON(&helloFrame); err != nil {
			_ = ws.Close()
			return
		}
		var helloReq HelloRequest
		if err := json.Unmarshal(helloFrame.Params, &helloReq); err != nil {
			_ = ws.Close()
			return
		}
		helloResp := NegotiateHello(helloReq, CurrentProtocolVersion, serverCaps)
		encoded, _ := json.Marshal(helloResp)
		if err := ws.WriteJSON(Frame{ID: helloFrame.ID, Result: encoded}); err != nil {
			_ = ws.Close()
			return
		}
		c := &tunnelTestHubConn{
			ws:      ws,
			pending: make(map[string]chan Frame),
			frames:  make(chan ExecTunnelFrameParams, 256),
			closes:  make(chan ExecTunnelCloseParams, 64),
			done:    make(chan struct{}),
		}
		go c.readLoop()
		conns <- c
	}))
	t.Cleanup(srv.Close)
	return srv, conns
}

func (c *tunnelTestHubConn) readLoop() {
	defer close(c.done)
	for {
		var frame Frame
		if err := c.ws.ReadJSON(&frame); err != nil {
			return
		}
		switch frame.Method {
		case MethodExecTunnelFrame:
			var p ExecTunnelFrameParams
			if json.Unmarshal(frame.Params, &p) == nil {
				c.frames <- p
			}
			continue
		case MethodExecTunnelClose:
			var p ExecTunnelCloseParams
			if json.Unmarshal(frame.Params, &p) == nil {
				c.closes <- p
			}
			continue
		}
		if frame.ID == "" {
			continue
		}
		c.pendMu.Lock()
		ch := c.pending[frame.ID]
		delete(c.pending, frame.ID)
		c.pendMu.Unlock()
		if ch != nil {
			ch <- frame
		}
	}
}

func (c *tunnelTestHubConn) Call(ctx context.Context, method string, params any) (Frame, error) {
	id := fmt.Sprintf("req-%d", c.nextID.Add(1))
	raw, err := json.Marshal(params)
	if err != nil {
		return Frame{}, err
	}
	ch := make(chan Frame, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()

	c.writeMu.Lock()
	err = c.ws.WriteJSON(Frame{ID: id, Method: method, Params: raw})
	c.writeMu.Unlock()
	if err != nil {
		return Frame{}, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-c.done:
		return Frame{}, errors.New("hub connection closed")
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

func (c *tunnelTestHubConn) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(Frame{Method: method, Params: raw})
}

func (c *tunnelTestHubConn) Close() error {
	return c.ws.Close()
}

type localWSEchoLauncher struct {
	mu       sync.Mutex
	endpoint string
	token    string
	stopped  []string
	stopAlls atomic.Int32
}

func (l *localWSEchoLauncher) Start(_ context.Context, req ExecStartRequest) (ExecStartResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return ExecStartResponse{
		SessionID: "sess-" + req.RunID,
		Endpoint:  l.endpoint,
		AuthToken: l.token,
	}, nil
}

func (l *localWSEchoLauncher) Stop(_ context.Context, req ExecStopRequest) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = append(l.stopped, req.SessionID)
	return nil
}

func (l *localWSEchoLauncher) StopAll() {
	l.stopAlls.Add(1)
}

func TestExecTunnelBidirectionalRelayMultiplexAndFailClosed(t *testing.T) {
	const wantToken = "cap-secret-token-xyz"
	var (
		localUpgrader = websocket.Upgrader{}
		localConns    atomic.Int32
		localClosed   = make(chan struct{}, 16)
		seenAuth      = make(chan string, 16)
	)
	localExecSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		seenAuth <- auth
		ws, err := localUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		localConns.Add(1)
		defer func() {
			localConns.Add(-1)
			_ = ws.Close()
			localClosed <- struct{}{}
		}()
		for {
			msgType, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage && string(payload) == "trigger-local-close" {
				_ = ws.WriteMessage(websocket.TextMessage, []byte("goodbye-before-close"))
				return
			}
			if err := ws.WriteMessage(msgType, payload); err != nil {
				return
			}
		}
	}))
	defer localExecSrv.Close()

	localWSURL := "ws" + strings.TrimPrefix(localExecSrv.URL, "http")
	launcher := &localWSEchoLauncher{
		endpoint: localWSURL,
		token:    wantToken,
	}

	hub, conns := startTunnelTestHub(t, Capabilities{FileStaging: true, LocalExec: true})
	dir := t.TempDir()
	client, err := NewClient(ClientConfig{
		CloudURL:     hub.URL,
		Token:        "hub-tok",
		DeviceID:     "mac-1",
		Capabilities: Capabilities{FileStaging: true, LocalExec: true},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadWrite, RootPath: dir}},
		ExecLauncher: launcher,
		MinBackoff:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- client.Run(ctx)
	}()

	conn := <-conns

	// Unknown session_id must fail with not_found.
	badOpen, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "tun-missing",
		SessionID: "no-such-session",
	})
	if err != nil || badOpen.Error == nil || badOpen.Error.Code != CodeNotFound {
		t.Fatalf("unknown session open = (%+v, %v), want %s", badOpen, err, CodeNotFound)
	}

	// Start exec session so execTunnelManager registers session -> {endpoint, authToken}.
	startFrame, err := conn.Call(ctx, MethodExecStart, ExecStartRequest{
		GrantID: "work",
		RunID:   "run-1",
	})
	if err != nil || startFrame.Error != nil {
		t.Fatalf("device_exec_start failed: err=%v rpcErr=%v", err, startFrame.Error)
	}
	var startResp ExecStartResponse
	if err := json.Unmarshal(startFrame.Result, &startResp); err != nil {
		t.Fatal(err)
	}
	if startResp.SessionID != "sess-run-1" {
		t.Fatalf("unexpected session_id %q", startResp.SessionID)
	}

	// Open two multiplexed tunnels to the same exec session.
	for _, tid := range []string{"tun-a", "tun-b"} {
		openFrame, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
			TunnelID:  tid,
			SessionID: startResp.SessionID,
		})
		if err != nil || openFrame.Error != nil {
			t.Fatalf("exec_tunnel_open(%s) failed: err=%v rpcErr=%v", tid, err, openFrame.Error)
		}
		var openResp ExecTunnelOpenResponse
		if err := json.Unmarshal(openFrame.Result, &openResp); err != nil || !openResp.OK {
			t.Fatalf("unexpected open response for %s: %s", tid, string(openFrame.Result))
		}
		if got := <-seenAuth; got != "Bearer "+wantToken {
			t.Fatalf("local exec server saw auth %q, want Bearer %s", got, wantToken)
		}
	}

	// Duplicate tunnel_id must fail with conflict.
	dupFrame, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "tun-a",
		SessionID: startResp.SessionID,
	})
	if err != nil || dupFrame.Error == nil || dupFrame.Error.Code != CodeConflict {
		t.Fatalf("duplicate tunnel_id = (%+v, %v), want %s", dupFrame, err, CodeConflict)
	}

	// Send ordered burst of text + binary + empty frames on tun-a and verify exact FIFO ordering.
	const burstCount = 20
	for i := range burstCount {
		msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`, i)
		if err := conn.Notify(MethodExecTunnelFrame, ExecTunnelFrameParams{
			TunnelID:    "tun-a",
			MessageType: ExecTunnelMessageText,
			Data:        msg,
		}); err != nil {
			t.Fatal(err)
		}
	}
	binPayload := []byte{0x00, 0x01, 0xfe, 0xff, 0x7f, 0x80}
	if err := conn.Notify(MethodExecTunnelFrame, ExecTunnelFrameParams{
		TunnelID:    "tun-b",
		MessageType: ExecTunnelMessageBinary,
		Data:        base64.StdEncoding.EncodeToString(binPayload),
	}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Notify(MethodExecTunnelFrame, ExecTunnelFrameParams{
		TunnelID:    "tun-b",
		MessageType: ExecTunnelMessageText,
		Data:        "",
	}); err != nil {
		t.Fatal(err)
	}

	var gotA []string
	var gotB []ExecTunnelFrameParams
	for len(gotA) < burstCount || len(gotB) < 2 {
		select {
		case f := <-conn.frames:
			switch f.TunnelID {
			case "tun-a":
				if f.MessageType != ExecTunnelMessageText {
					t.Fatalf("tun-a expected text frame, got %+v", f)
				}
				gotA = append(gotA, f.Data)
			case "tun-b":
				gotB = append(gotB, f)
			default:
				t.Fatalf("unexpected tunnel frame %+v", f)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for tunnel frames: gotA=%d gotB=%d", len(gotA), len(gotB))
		}
	}
	for i := range burstCount {
		want := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`, i)
		if gotA[i] != want {
			t.Fatalf("tun-a frame[%d] = %q, want %q (ordering broken)", i, gotA[i], want)
		}
	}
	if gotB[0].MessageType != ExecTunnelMessageBinary {
		t.Fatalf("tun-b frame[0] type = %q, want binary", gotB[0].MessageType)
	}
	decodedBin, err := base64.StdEncoding.DecodeString(gotB[0].Data)
	if err != nil || !bytes.Equal(decodedBin, binPayload) {
		t.Fatalf("tun-b binary mismatch: decoded=%x err=%v", decodedBin, err)
	}
	if gotB[1].MessageType != ExecTunnelMessageText || gotB[1].Data != "" {
		t.Fatalf("tun-b empty frame mismatch: %+v", gotB[1])
	}

	// Local exec-server closing tun-b must emit final frame followed by exec_tunnel_close.
	if err := conn.Notify(MethodExecTunnelFrame, ExecTunnelFrameParams{
		TunnelID:    "tun-b",
		MessageType: ExecTunnelMessageText,
		Data:        "trigger-local-close",
	}); err != nil {
		t.Fatal(err)
	}
	var sawGoodbye bool
	var sawCloseB bool
	for !sawGoodbye || !sawCloseB {
		select {
		case f := <-conn.frames:
			if f.TunnelID == "tun-b" && f.Data == "goodbye-before-close" {
				sawGoodbye = true
			}
		case cl := <-conn.closes:
			if cl.TunnelID == "tun-b" {
				sawCloseB = true
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for local close: sawGoodbye=%v sawCloseB=%v", sawGoodbye, sawCloseB)
		}
	}
	<-localClosed

	// Oversized frame on tun-a (> MaxExecTunnelFrameBytes) closes tun-a without killing the hub WS.
	oversized := strings.Repeat("x", MaxExecTunnelFrameBytes+1)
	if err := conn.Notify(MethodExecTunnelFrame, ExecTunnelFrameParams{
		TunnelID:    "tun-a",
		MessageType: ExecTunnelMessageText,
		Data:        oversized,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case cl := <-conn.closes:
		if cl.TunnelID != "tun-a" {
			t.Fatalf("expected close for tun-a, got %+v", cl)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for oversized frame close")
	}
	<-localClosed

	// Hub WS is still healthy: open tun-c, then stop session via device_exec_stop.
	openC, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "tun-c",
		SessionID: startResp.SessionID,
	})
	if err != nil || openC.Error != nil {
		t.Fatalf("exec_tunnel_open(tun-c) failed: err=%v rpcErr=%v", err, openC.Error)
	}
	<-seenAuth

	stopFrame, err := conn.Call(ctx, MethodExecStop, ExecStopRequest{SessionID: startResp.SessionID})
	if err != nil || stopFrame.Error != nil {
		t.Fatalf("device_exec_stop failed: err=%v rpcErr=%v", err, stopFrame.Error)
	}
	select {
	case cl := <-conn.closes:
		if cl.TunnelID != "tun-c" {
			t.Fatalf("expected exec_tunnel_close for tun-c on stop, got %+v", cl)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for tun-c close on device_exec_stop")
	}
	<-localClosed

	// After device_exec_stop, reopening a tunnel for the stopped session must fail with not_found.
	reopen, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "tun-after-stop",
		SessionID: startResp.SessionID,
	})
	if err != nil || reopen.Error == nil || reopen.Error.Code != CodeNotFound {
		t.Fatalf("reopen after stop = (%+v, %v), want %s", reopen, err, CodeNotFound)
	}

	// Start a new session + open tun-d, then drop the hub WS: must fail-close local WS and StopAll.
	start2, err := conn.Call(ctx, MethodExecStart, ExecStartRequest{
		GrantID: "work",
		RunID:   "run-2",
	})
	if err != nil || start2.Error != nil {
		t.Fatalf("second device_exec_start failed: err=%v rpcErr=%v", err, start2.Error)
	}
	openD, err := conn.Call(ctx, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "tun-d",
		SessionID: "sess-run-2",
	})
	if err != nil || openD.Error != nil {
		t.Fatalf("exec_tunnel_open(tun-d) failed: err=%v rpcErr=%v", err, openD.Error)
	}
	<-seenAuth

	_ = conn.Close()
	select {
	case <-localClosed:
	case <-ctx.Done():
		t.Fatal("expected local exec WS tunnel to close immediately on hub disconnect")
	}
	// Wait for reconnect to confirm disconnect cleanup (including StopAll) completed.
	conn2 := <-conns
	if launcher.stopAlls.Load() < 1 {
		t.Fatalf("expected StopAll on disconnect, got %d", launcher.stopAlls.Load())
	}
	_ = conn2.Close()

	cancel()
	<-runDone
}

func TestExecTunnelRejectsNonLoopbackAndDisabledLocalExec(t *testing.T) {
	dir := t.TempDir()

	// 1. LocalExec disabled rejects exec_tunnel_open with unsupported_capability.
	hub1, conns1 := startTunnelTestHub(t, Capabilities{FileStaging: true, LocalExec: false})
	client1, err := NewClient(ClientConfig{
		CloudURL:     hub1.URL,
		Token:        "tok",
		DeviceID:     "mac-noexec",
		Capabilities: Capabilities{FileStaging: true, LocalExec: false},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadWrite, RootPath: dir}},
		MinBackoff:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx1, cancel1 := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel1()
	done1 := make(chan error, 1)
	go func() {
		done1 <- client1.Run(ctx1)
	}()
	c1 := <-conns1
	resp1, err := c1.Call(ctx1, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "t1",
		SessionID: "s1",
	})
	if err != nil || resp1.Error == nil || resp1.Error.Code != CodeUnsupportedCapability {
		t.Fatalf("disabled local_exec open = (%+v, %v), want %s", resp1, err, CodeUnsupportedCapability)
	}
	cancel1()
	<-done1

	// 2. Non-loopback endpoint returned by a buggy launcher is refused before dialing.
	launcher := &localWSEchoLauncher{
		endpoint: "ws://example.com:9999/exec",
		token:    "tok",
	}
	hub2, conns2 := startTunnelTestHub(t, Capabilities{LocalExec: true})
	client2, err := NewClient(ClientConfig{
		CloudURL:     hub2.URL,
		Token:        "tok",
		DeviceID:     "mac-ssrf",
		Capabilities: Capabilities{LocalExec: true},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadWrite, RootPath: dir}},
		ExecLauncher: launcher,
		MinBackoff:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel2()
	done2 := make(chan error, 1)
	go func() {
		done2 <- client2.Run(ctx2)
	}()
	c2 := <-conns2
	startResp, err := c2.Call(ctx2, MethodExecStart, ExecStartRequest{
		GrantID: "work",
		RunID:   "bad-host",
	})
	if err != nil || startResp.Error != nil {
		t.Fatalf("ExecStart failed: %+v %v", startResp, err)
	}
	openResp, err := c2.Call(ctx2, MethodExecTunnelOpen, ExecTunnelOpenRequest{
		TunnelID:  "t-ssrf",
		SessionID: "sess-bad-host",
	})
	if err != nil || openResp.Error == nil || openResp.Error.Code != CodeInvalidRequest {
		t.Fatalf("non-loopback open = (%+v, %v), want %s", openResp, err, CodeInvalidRequest)
	}
	cancel2()
	<-done2
}
