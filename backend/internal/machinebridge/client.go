package machinebridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	connectPath           = "/v1/machines/connect"
	defaultMaxConcurrency = 8
	defaultPingInterval   = 25 * time.Second
	defaultPongWait       = 75 * time.Second
	defaultWriteTimeout   = 15 * time.Second
	defaultMinBackoff     = 1 * time.Second
	defaultMaxBackoff     = 30 * time.Second
	defaultReadLimit      = 4 << 20
	helloFrameID          = "hello"
)

type ClientConfig struct {
	CloudURL               string
	Token                  string
	DeviceID               string
	DisplayName            string
	Platform               string
	AppVersion             string
	Capabilities           Capabilities
	FolderGrants           []FolderGrant
	HTTPClient             *http.Client
	Dialer                 *websocket.Dialer
	ExecLauncher           ExecLauncher
	ExecLauncherFactory    func(*PolicyGuard) (ExecLauncher, error)
	AllowedTransferHosts   []string
	AllowLoopbackTransfers bool
	MaxConcurrency         int
	PingInterval           time.Duration
	PongWait               time.Duration
	WriteTimeout           time.Duration
	MinBackoff             time.Duration
	MaxBackoff             time.Duration
	OnConnect              func(Capabilities)
	OnDisconnect           func(error)
}

type Client struct {
	cfg          ClientConfig
	connectURL   string
	guard        *PolicyGuard
	execLauncher ExecLauncher
	handler      *Handler
	dialer       *websocket.Dialer

	mu         sync.RWMutex
	negotiated Capabilities
}

func NewClient(cfg ClientConfig) (*Client, error) {
	connectURL, err := BuildConnectURL(cfg.CloudURL)
	if err != nil {
		return nil, err
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	if cfg.Token == "" {
		return nil, fmt.Errorf("%w: token is required", ErrInvalidRequest)
	}
	cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)
	if cfg.DeviceID == "" {
		return nil, fmt.Errorf("%w: device_id is required", ErrInvalidRequest)
	}
	cfg.DisplayName = strings.TrimSpace(cfg.DisplayName)
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.DeviceID
	}
	cfg.Platform = strings.TrimSpace(cfg.Platform)
	if cfg.Platform == "" {
		cfg.Platform = runtime.GOOS
	}
	if !ValidPlatform(cfg.Platform) {
		return nil, fmt.Errorf("%w: unsupported platform %q", ErrInvalidRequest, cfg.Platform)
	}
	var guardOpts []PolicyOption
	if cfg.AllowLoopbackTransfers {
		guardOpts = append(guardOpts, WithAllowLoopbackTransfers())
	}
	allowedHosts := cfg.AllowedTransferHosts
	if len(allowedHosts) == 0 {
		allowedHosts = []string{defaultTransferHost}
		if parsedConnect, parseErr := url.Parse(connectURL); parseErr == nil {
			if cloudHost := normalizeTransferHost(parsedConnect.Hostname()); cloudHost != "" && cloudHost != defaultTransferHost {
				allowedHosts = append(allowedHosts, cloudHost)
			}
		}
	}
	guardOpts = append(guardOpts, WithAllowedTransferHosts(allowedHosts...))
	guard, err := NewPolicyGuard(cfg.FolderGrants, guardOpts...)
	if err != nil {
		return nil, err
	}
	execLauncher := cfg.ExecLauncher
	if execLauncher == nil && cfg.ExecLauncherFactory != nil {
		execLauncher, err = cfg.ExecLauncherFactory(guard)
		if err != nil {
			_ = guard.Close()
			return nil, err
		}
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = defaultMaxConcurrency
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = defaultPingInterval
	}
	if cfg.PongWait <= 0 {
		cfg.PongWait = defaultPongWait
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = defaultMinBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	if cfg.MaxBackoff < cfg.MinBackoff {
		cfg.MaxBackoff = cfg.MinBackoff
	}
	dialer := cfg.Dialer
	if dialer == nil {
		copied := *websocket.DefaultDialer
		dialer = &copied
	}
	return &Client{
		cfg:          cfg,
		connectURL:   connectURL,
		guard:        guard,
		execLauncher: execLauncher,
		handler:      NewHandler(guard, cfg.HTTPClient, execLauncher),
		dialer:       dialer,
	}, nil
}

func BuildConnectURL(cloudURL string) (string, error) {
	raw := strings.TrimSpace(cloudURL)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: valid cloud URL is required", ErrInvalidRequest)
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "wss":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return "", fmt.Errorf("%w: non-loopback cloud URL must use https or wss", ErrInvalidRequest)
		}
		parsed.Scheme = "ws"
	case "ws":
		if !isLoopbackHost(parsed.Hostname()) {
			return "", fmt.Errorf("%w: non-loopback cloud URL must use https or wss", ErrInvalidRequest)
		}
	default:
		return "", fmt.Errorf("%w: unsupported cloud URL scheme %q", ErrInvalidRequest, parsed.Scheme)
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(trimmedPath, connectPath) {
		parsed.Path = trimmedPath + connectPath
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c *Client) NegotiatedCapabilities() Capabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.negotiated
}

func (c *Client) Close() error {
	var errs []error
	if closer, ok := c.execLauncher.(io.Closer); ok {
		errs = append(errs, closer.Close())
	}
	if c.guard != nil {
		errs = append(errs, c.guard.Close())
	}
	return errors.Join(errs...)
}

func (c *Client) stopExecSessions() {
	if stopper, ok := c.execLauncher.(interface{ StopAll() }); ok {
		stopper.StopAll()
	}
}

func (c *Client) Run(ctx context.Context) error {
	defer func() { _ = c.Close() }()
	backoff := c.cfg.MinBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		connected, err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.cfg.OnDisconnect != nil {
			c.cfg.OnDisconnect(err)
		}
		if connected {
			backoff = c.cfg.MinBackoff
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff = min(backoff*2, c.cfg.MaxBackoff)
	}
}

func (c *Client) connectAndServe(ctx context.Context) (bool, error) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+c.cfg.Token)

	ws, resp, err := c.dialer.DialContext(ctx, c.connectURL, headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return false, fmt.Errorf("dial %s: %w", c.connectURL, err)
	}

	ws.SetReadLimit(defaultReadLimit)
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	var writeMu sync.Mutex
	writeFrame := func(frame Frame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = ws.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
		return ws.WriteJSON(frame)
	}

	var bgWG sync.WaitGroup
	var closeOnce sync.Once
	closeWS := func() {
		closeOnce.Do(func() {
			_ = ws.Close()
		})
	}
	closeStop := make(chan struct{})
	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		select {
		case <-sessionCtx.Done():
			_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			closeWS()
		case <-closeStop:
		}
	}()
	defer func() {
		close(closeStop)
		closeWS()
		bgWG.Wait()
	}()

	negotiated, err := c.handshake(ws, writeFrame)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.negotiated = negotiated
	c.mu.Unlock()
	if c.cfg.OnConnect != nil {
		c.cfg.OnConnect(negotiated)
	}

	_ = ws.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
	})
	defaultPing := ws.PingHandler()
	ws.SetPingHandler(func(appData string) error {
		_ = ws.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
		return defaultPing(appData)
	})

	pingDone := make(chan struct{})
	bgWG.Add(1)
	go func() {
		defer bgWG.Done()
		ticker := time.NewTicker(c.cfg.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-sessionCtx.Done():
				return
			case <-ticker.C:
				if ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
					closeWS()
					return
				}
			}
		}
	}()

	tunnels := newExecTunnelManager(sessionCtx, writeFrame, cancelSession)
	sem := make(chan struct{}, c.cfg.MaxConcurrency)
	var wg sync.WaitGroup
	var readErr error
readLoop:
	for {
		var frame Frame
		if err := ws.ReadJSON(&frame); err != nil {
			readErr = err
			break
		}
		_ = ws.SetReadDeadline(time.Now().Add(c.cfg.PongWait))
		if frame.Method == "" {
			continue
		}
		switch frame.Method {
		case MethodExecTunnelFrame:
			tunnels.HandleFrame(frame.Params)
			continue
		case MethodExecTunnelClose:
			tunnels.HandleClose(frame.Params)
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-sessionCtx.Done():
			readErr = sessionCtx.Err()
			break readLoop
		}
		wg.Add(1)
		go func(req Frame) {
			defer wg.Done()
			defer func() { <-sem }()
			if req.Method == MethodExecTunnelOpen {
				tun, reply := tunnels.HandleOpen(sessionCtx, negotiated, req)
				if err := writeFrame(reply); err != nil {
					if tun != nil {
						tunnels.CloseTunnel(tun.id, "hub write failed", false)
					}
					cancelSession()
					return
				}
				if tun != nil {
					tun.start()
				}
				return
			}
			reply := c.handleFrame(sessionCtx, negotiated, tunnels, req)
			if err := writeFrame(reply); err != nil {
				cancelSession()
			}
		}(frame)
	}

	cancelSession()
	tunnels.CloseAll()
	close(pingDone)
	wg.Wait()
	tunnels.Wait()
	c.stopExecSessions()
	return true, readErr
}

func (c *Client) handshake(ws *websocket.Conn, writeFrame func(Frame) error) (Capabilities, error) {
	helloParams, err := json.Marshal(HelloRequest{
		ProtocolVersion: CurrentProtocolVersion,
		DeviceID:        c.cfg.DeviceID,
		DisplayName:     c.cfg.DisplayName,
		Platform:        c.cfg.Platform,
		AppVersion:      c.cfg.AppVersion,
		Capabilities:    c.cfg.Capabilities,
		FolderGrants:    c.guard.Grants(),
	})
	if err != nil {
		return Capabilities{}, err
	}
	if err := writeFrame(Frame{
		ID:     helloFrameID,
		Method: MethodHello,
		Params: helloParams,
	}); err != nil {
		return Capabilities{}, fmt.Errorf("send hello: %w", err)
	}

	_ = ws.SetReadDeadline(time.Now().Add(c.cfg.WriteTimeout))
	var reply Frame
	if err := ws.ReadJSON(&reply); err != nil {
		return Capabilities{}, fmt.Errorf("read hello response: %w", err)
	}
	if reply.ID != "" && reply.ID != helloFrameID {
		return Capabilities{}, fmt.Errorf("unexpected hello response id %q", reply.ID)
	}
	if reply.Error != nil {
		return Capabilities{}, fmt.Errorf("hello rejected: %w", reply.Error)
	}
	var resp HelloResponse
	if err := json.Unmarshal(reply.Result, &resp); err != nil {
		return Capabilities{}, fmt.Errorf("decode hello response: %w", err)
	}
	if !resp.Accepted {
		msg := strings.TrimSpace(resp.Error)
		if msg == "" {
			msg = "hello rejected by server"
		}
		return Capabilities{}, errors.New(msg)
	}
	return NegotiateCapabilities(c.cfg.Capabilities, resp.Capabilities), nil
}

func (c *Client) handleFrame(ctx context.Context, caps Capabilities, tunnels *execTunnelManager, req Frame) Frame {
	result, err := c.handler.Dispatch(ctx, caps, req.Method, req.Params)
	if req.Method == MethodExecStop && (err == nil || errors.Is(err, ErrNotFound)) {
		var stopReq ExecStopRequest
		if json.Unmarshal(req.Params, &stopReq) == nil {
			tunnels.RemoveSession(stopReq.SessionID, "exec session stopped")
		}
	}
	if err != nil {
		return Frame{ID: req.ID, Error: toRPCError(err)}
	}
	if req.Method == MethodExecStart {
		if resp, ok := result.(ExecStartResponse); ok {
			tunnels.RegisterSession(resp.SessionID, resp.Endpoint, resp.AuthToken)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return Frame{ID: req.ID, Error: &RPCError{Code: CodeInternal, Message: err.Error()}}
	}
	return Frame{ID: req.ID, Result: encoded}
}
