package machinebridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxExecTunnelsPerConn  = 32
	execTunnelQueueLimit   = 512
	execTunnelDialTimeout  = 10 * time.Second
	execTunnelWriteTimeout = 15 * time.Second
)

type execSessionTarget struct {
	endpoint  string
	authToken string
}

type execTunnelMsg struct {
	wsType int
	data   []byte
}

type execTunnelManager struct {
	sessionCtx    context.Context
	writeFrame    func(Frame) error
	cancelSession func()
	localDialer   *websocket.Dialer

	mu       sync.Mutex
	closed   bool
	sessions map[string]execSessionTarget
	tunnels  map[string]*deviceExecTunnel
	wg       sync.WaitGroup
}

func newExecTunnelManager(sessionCtx context.Context, writeFrame func(Frame) error, cancelSession func()) *execTunnelManager {
	return &execTunnelManager{
		sessionCtx:    sessionCtx,
		writeFrame:    writeFrame,
		cancelSession: cancelSession,
		localDialer: &websocket.Dialer{
			HandshakeTimeout: execTunnelDialTimeout,
		},
		sessions: make(map[string]execSessionTarget),
		tunnels:  make(map[string]*deviceExecTunnel),
	}
}

func (m *execTunnelManager) RegisterSession(sessionID, endpoint, authToken string) {
	sessionID = strings.TrimSpace(sessionID)
	endpoint = strings.TrimSpace(endpoint)
	if sessionID == "" || endpoint == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.sessions[sessionID] = execSessionTarget{
		endpoint:  endpoint,
		authToken: authToken,
	}
}

func (m *execTunnelManager) RemoveSession(sessionID, reason string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, sessionID)
	var victims []*deviceExecTunnel
	for id, tun := range m.tunnels {
		if tun.sessionID == sessionID {
			delete(m.tunnels, id)
			victims = append(victims, tun)
		}
	}
	m.mu.Unlock()

	for _, tun := range victims {
		tun.abort(reason, true)
	}
}

func (m *execTunnelManager) Open(ctx context.Context, caps Capabilities, req ExecTunnelOpenRequest) (*deviceExecTunnel, error) {
	if !caps.LocalExec {
		return nil, fmt.Errorf("%w: local exec disabled", ErrUnsupportedCapability)
	}
	tunnelID := strings.TrimSpace(req.TunnelID)
	sessionID := strings.TrimSpace(req.SessionID)
	if tunnelID == "" || sessionID == "" {
		return nil, fmt.Errorf("%w: tunnel_id and session_id are required", ErrInvalidRequest)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrDeviceOffline
	}
	if _, exists := m.tunnels[tunnelID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: duplicate tunnel_id %q", ErrConflict, tunnelID)
	}
	if len(m.tunnels) >= maxExecTunnelsPerConn {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: too many active exec tunnels", ErrConflict)
	}
	target, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: unknown exec session %q", ErrNotFound, sessionID)
	}
	if err := validateLocalExecEndpoint(target.endpoint); err != nil {
		return nil, err
	}

	headers := http.Header{}
	if target.authToken != "" {
		headers.Set("Authorization", "Bearer "+target.authToken)
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, execTunnelDialTimeout)
	defer cancelDial()

	conn, resp, err := m.localDialer.DialContext(dialCtx, target.endpoint, headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("dial local exec session %q: %w", sessionID, err)
	}
	conn.SetReadLimit(MaxExecTunnelFrameBytes)

	tun := newDeviceExecTunnel(m, tunnelID, sessionID, conn)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = conn.Close()
		return nil, ErrDeviceOffline
	}
	if _, exists := m.tunnels[tunnelID]; exists {
		m.mu.Unlock()
		_ = conn.Close()
		return nil, fmt.Errorf("%w: duplicate tunnel_id %q", ErrConflict, tunnelID)
	}
	if len(m.tunnels) >= maxExecTunnelsPerConn {
		m.mu.Unlock()
		_ = conn.Close()
		return nil, fmt.Errorf("%w: too many active exec tunnels", ErrConflict)
	}
	if _, stillExists := m.sessions[sessionID]; !stillExists {
		m.mu.Unlock()
		_ = conn.Close()
		return nil, fmt.Errorf("%w: unknown exec session %q", ErrNotFound, sessionID)
	}
	m.tunnels[tunnelID] = tun
	m.mu.Unlock()

	return tun, nil
}

func (m *execTunnelManager) HandleOpen(ctx context.Context, caps Capabilities, req Frame) (*deviceExecTunnel, Frame) {
	var openReq ExecTunnelOpenRequest
	if err := decodeParams(req.Params, &openReq); err != nil {
		return nil, Frame{ID: req.ID, Error: toRPCError(err)}
	}
	tun, err := m.Open(ctx, caps, openReq)
	if err != nil {
		return nil, Frame{ID: req.ID, Error: toRPCError(err)}
	}
	encoded, err := json.Marshal(ExecTunnelOpenResponse{OK: true})
	if err != nil {
		m.CloseTunnel(tun.id, "encode open response failed", false)
		return nil, Frame{ID: req.ID, Error: &RPCError{Code: CodeInternal, Message: err.Error()}}
	}
	return tun, Frame{ID: req.ID, Result: encoded}
}

func (m *execTunnelManager) HandleFrame(raw json.RawMessage) {
	var params ExecTunnelFrameParams
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	tunnelID := strings.TrimSpace(params.TunnelID)
	if tunnelID == "" {
		return
	}
	m.mu.Lock()
	tun := m.tunnels[tunnelID]
	m.mu.Unlock()
	if tun == nil {
		return
	}
	msg, err := decodeExecTunnelFrame(params)
	if err != nil {
		m.removeTunnel(tunnelID, tun)
		tun.abort(err.Error(), true)
		return
	}
	if !tun.enqueue(msg) {
		m.removeTunnel(tunnelID, tun)
		tun.abort("exec tunnel backpressure limit exceeded", true)
	}
}

func (m *execTunnelManager) HandleClose(raw json.RawMessage) {
	var params ExecTunnelCloseParams
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	tunnelID := strings.TrimSpace(params.TunnelID)
	if tunnelID == "" {
		return
	}
	m.mu.Lock()
	tun := m.tunnels[tunnelID]
	if tun != nil {
		delete(m.tunnels, tunnelID)
	}
	m.mu.Unlock()
	if tun != nil {
		tun.closeGraceful()
	}
}

func (m *execTunnelManager) CloseTunnel(tunnelID, reason string, notifyPeer bool) {
	m.mu.Lock()
	tun := m.tunnels[tunnelID]
	if tun != nil {
		delete(m.tunnels, tunnelID)
	}
	m.mu.Unlock()
	if tun != nil {
		tun.abort(reason, notifyPeer)
	}
}

func (m *execTunnelManager) CloseAll() {
	m.mu.Lock()
	m.closed = true
	clear(m.sessions)
	victims := make([]*deviceExecTunnel, 0, len(m.tunnels))
	for id, tun := range m.tunnels {
		victims = append(victims, tun)
		delete(m.tunnels, id)
	}
	m.mu.Unlock()

	for _, tun := range victims {
		tun.abort("hub disconnected", false)
	}
}

func (m *execTunnelManager) Wait() {
	m.wg.Wait()
}

func (m *execTunnelManager) removeTunnel(tunnelID string, tun *deviceExecTunnel) {
	m.mu.Lock()
	if m.tunnels[tunnelID] == tun {
		delete(m.tunnels, tunnelID)
	}
	m.mu.Unlock()
}

func (m *execTunnelManager) sendCloseFrame(tunnelID, reason string) {
	if m.sessionCtx.Err() != nil {
		return
	}
	raw, err := json.Marshal(ExecTunnelCloseParams{
		TunnelID: tunnelID,
		Reason:   reason,
	})
	if err != nil {
		return
	}
	if err := m.writeFrame(Frame{
		Method: MethodExecTunnelClose,
		Params: raw,
	}); err != nil {
		m.cancelSession()
	}
}

type deviceExecTunnel struct {
	mgr       *execTunnelManager
	id        string
	sessionID string
	conn      *websocket.Conn

	mu        sync.Mutex
	started   bool
	eof       bool
	aborted   bool
	closeSent bool
	queue     []execTunnelMsg
	notify    chan struct{}
}

func newDeviceExecTunnel(mgr *execTunnelManager, id, sessionID string, conn *websocket.Conn) *deviceExecTunnel {
	return &deviceExecTunnel{
		mgr:       mgr,
		id:        id,
		sessionID: sessionID,
		conn:      conn,
		notify:    make(chan struct{}, 1),
	}
}

func (t *deviceExecTunnel) start() {
	t.mu.Lock()
	if t.started || t.aborted {
		t.mu.Unlock()
		return
	}
	t.started = true
	t.mgr.wg.Add(2)
	t.mu.Unlock()

	go t.readPump()
	go t.writePump()
}

func (t *deviceExecTunnel) enqueue(msg execTunnelMsg) bool {
	t.mu.Lock()
	if t.eof || t.aborted || len(t.queue) >= execTunnelQueueLimit {
		t.mu.Unlock()
		return false
	}
	t.queue = append(t.queue, msg)
	t.signalLocked()
	t.mu.Unlock()
	return true
}

func (t *deviceExecTunnel) closeGraceful() {
	t.mu.Lock()
	t.eof = true
	t.closeSent = true
	closeNow := !t.started && len(t.queue) == 0
	if closeNow {
		t.aborted = true
	}
	t.signalLocked()
	t.mu.Unlock()
	if closeNow {
		_ = t.conn.Close()
	}
}

func (t *deviceExecTunnel) abort(reason string, notifyPeer bool) {
	t.mu.Lock()
	t.aborted = true
	t.queue = nil
	shouldNotify := notifyPeer && !t.closeSent
	if shouldNotify {
		t.closeSent = true
	}
	t.signalLocked()
	t.mu.Unlock()

	_ = t.conn.Close()
	if shouldNotify {
		t.mgr.sendCloseFrame(t.id, reason)
	}
}

func (t *deviceExecTunnel) signalLocked() {
	select {
	case t.notify <- struct{}{}:
	default:
	}
}

func (t *deviceExecTunnel) pop() (execTunnelMsg, bool) {
	for {
		t.mu.Lock()
		if t.aborted {
			t.mu.Unlock()
			return execTunnelMsg{}, false
		}
		if len(t.queue) > 0 {
			msg := t.queue[0]
			t.queue[0] = execTunnelMsg{}
			t.queue = t.queue[1:]
			if len(t.queue) == 0 {
				t.queue = t.queue[:0]
			}
			t.mu.Unlock()
			return msg, true
		}
		if t.eof {
			t.mu.Unlock()
			return execTunnelMsg{}, false
		}
		t.mu.Unlock()

		select {
		case <-t.notify:
		case <-t.mgr.sessionCtx.Done():
			return execTunnelMsg{}, false
		}
	}
}

func (t *deviceExecTunnel) writePump() {
	defer t.mgr.wg.Done()
	for {
		msg, ok := t.pop()
		if !ok {
			t.mu.Lock()
			graceful := t.eof && !t.aborted
			t.mu.Unlock()
			if graceful {
				_ = t.conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(time.Second),
				)
			}
			_ = t.conn.Close()
			return
		}
		_ = t.conn.SetWriteDeadline(time.Now().Add(execTunnelWriteTimeout))
		if err := t.conn.WriteMessage(msg.wsType, msg.data); err != nil {
			t.mgr.removeTunnel(t.id, t)
			t.abort("local exec write failed", true)
			return
		}
	}
}

func (t *deviceExecTunnel) readPump() {
	defer t.mgr.wg.Done()
	for {
		msgType, payload, err := t.conn.ReadMessage()
		if err != nil {
			reason := "local exec closed"
			if errors.Is(err, websocket.ErrReadLimit) {
				reason = "frame size limit exceeded"
			}
			t.mgr.removeTunnel(t.id, t)
			t.abort(reason, true)
			return
		}
		params, err := encodeExecTunnelFrame(t.id, msgType, payload)
		if err != nil {
			t.mgr.removeTunnel(t.id, t)
			t.abort(err.Error(), true)
			return
		}
		raw, err := json.Marshal(params)
		if err != nil {
			t.mgr.removeTunnel(t.id, t)
			t.abort("encode tunnel frame failed", true)
			return
		}
		if err := t.mgr.writeFrame(Frame{
			Method: MethodExecTunnelFrame,
			Params: raw,
		}); err != nil {
			t.mgr.removeTunnel(t.id, t)
			t.abort("hub write failed", false)
			t.mgr.cancelSession()
			return
		}
	}
}

func validateLocalExecEndpoint(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "ws" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%w: local exec endpoint must be loopback ws://", ErrInvalidRequest)
	}
	if !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("%w: local exec endpoint host %q is not loopback", ErrInvalidRequest, parsed.Hostname())
	}
	return nil
}

func encodeExecTunnelFrame(tunnelID string, wsType int, payload []byte) (ExecTunnelFrameParams, error) {
	if len(payload) > MaxExecTunnelFrameBytes {
		return ExecTunnelFrameParams{}, fmt.Errorf("%w: frame size limit exceeded", ErrInvalidRequest)
	}
	switch wsType {
	case websocket.TextMessage:
		return ExecTunnelFrameParams{
			TunnelID:    tunnelID,
			MessageType: ExecTunnelMessageText,
			Data:        string(payload),
		}, nil
	case websocket.BinaryMessage:
		return ExecTunnelFrameParams{
			TunnelID:    tunnelID,
			MessageType: ExecTunnelMessageBinary,
			Data:        base64.StdEncoding.EncodeToString(payload),
		}, nil
	default:
		return ExecTunnelFrameParams{}, fmt.Errorf("%w: unsupported websocket message type %d", ErrInvalidRequest, wsType)
	}
}

func decodeExecTunnelFrame(params ExecTunnelFrameParams) (execTunnelMsg, error) {
	switch params.MessageType {
	case ExecTunnelMessageText:
		if len(params.Data) > MaxExecTunnelFrameBytes {
			return execTunnelMsg{}, fmt.Errorf("%w: frame size limit exceeded", ErrInvalidRequest)
		}
		return execTunnelMsg{
			wsType: websocket.TextMessage,
			data:   []byte(params.Data),
		}, nil
	case ExecTunnelMessageBinary:
		if len(params.Data) > base64.StdEncoding.EncodedLen(MaxExecTunnelFrameBytes) {
			return execTunnelMsg{}, fmt.Errorf("%w: frame size limit exceeded", ErrInvalidRequest)
		}
		decoded, err := base64.StdEncoding.DecodeString(params.Data)
		if err != nil {
			return execTunnelMsg{}, fmt.Errorf("%w: invalid base64 frame payload", ErrInvalidRequest)
		}
		if len(decoded) > MaxExecTunnelFrameBytes {
			return execTunnelMsg{}, fmt.Errorf("%w: frame size limit exceeded", ErrInvalidRequest)
		}
		return execTunnelMsg{
			wsType: websocket.BinaryMessage,
			data:   decoded,
		}, nil
	default:
		return execTunnelMsg{}, fmt.Errorf("%w: unsupported message_type %q", ErrInvalidRequest, params.MessageType)
	}
}
