package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

type AuthorizationHook = func(context.Context) error

// Authorize runs the interactive OAuth authorization-code flow for a server,
// persists the resulting token, and reconnects so the server's tools become
// available. Browser clients receive the provider URL immediately and finish
// through the registered callback.
func (m *Manager) Authorize(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	return m.authorize(ctx, server, opts, nil)
}

func (m *Manager) AuthorizeWithHook(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	return m.authorize(ctx, server, opts, onAuthorized)
}

func (m *Manager) authorize(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	if m.ctx.Err() != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "MCP manager is closed", CheckedAt: time.Now().UTC()}
	}
	if server.Transport == mcpconfig.TransportStdio {
		return mcpconfig.ServerStatus{Status: "error", Error: "Local stdio servers do not use OAuth sign-in", CheckedAt: time.Now().UTC()}
	}
	if server.Transport == mcpconfig.TransportSSE {
		return mcpconfig.ServerStatus{Status: "error", Error: "Legacy SSE OAuth sign-in is not supported; use configured headers or Streamable HTTP", CheckedAt: time.Now().UTC()}
	}
	if m.tokens == nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "token store is not configured", CheckedAt: time.Now().UTC()}
	}
	loopback := server.OAuth.UsesLoopback()
	if loopback && !opts.LoopbackAllowed {
		return mcpconfig.ServerStatus{Status: "error", Error: "Public OAuth sign-in requires a local Jaz backend on this device", CheckedAt: time.Now().UTC()}
	}
	redirectURL := strings.TrimSpace(opts.RedirectURL)
	if redirectURL == "" && !loopback {
		return mcpconfig.ServerStatus{Status: "error", Error: "OAuth redirect URL is not configured", CheckedAt: time.Now().UTC()}
	}
	started := make(chan string, 1)
	pending := newAuthorizationPending()
	receiver := &callbackReceiver{
		manager: m,
		started: started,
		pending: pending,
	}
	flowCtx, cancel := context.WithCancel(m.ctx)
	pending.cancel = cancel
	tokenID := server.TokenID()
	m.authMu.Lock()
	if err := cmp.Or(ctx.Err(), flowCtx.Err()); err != nil {
		m.authMu.Unlock()
		cancel()
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	if active := m.authFlows[tokenID]; active != nil {
		if !opts.ReplacePending || !active.claimCallback() {
			m.authMu.Unlock()
			cancel()
			return mcpconfig.ServerStatus{Status: "error", Error: "Sign-in is already in progress for this server", CheckedAt: time.Now().UTC()}
		}
		active.cancel()
	}
	m.authFlows[tokenID] = pending
	m.authMu.Unlock()
	go func() {
		defer cancel()
		status := receiver.authorize(flowCtx, server, redirectURL, onAuthorized)
		m.authMu.Lock()
		if m.authFlows[tokenID] == pending {
			m.setServerStatus(server.ID, status)
			delete(m.authFlows, tokenID)
		}
		m.authMu.Unlock()
		pending.complete(status)
	}()

	select {
	case start := <-started:
		if err := flowCtx.Err(); err != nil {
			return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
		}
		return mcpconfig.ServerStatus{Status: "needs_auth", Error: "Sign in required", AuthURL: start, CheckedAt: time.Now().UTC()}
	case <-pending.done:
		return pending.status
	case <-ctx.Done():
		cancel()
		return mcpconfig.ServerStatus{Status: "error", Error: ctx.Err().Error(), CheckedAt: time.Now().UTC()}
	}
}

func (m *Manager) runAuthorize(ctx context.Context, server mcpconfig.Server, redirectURL string, fetch codeFetcher, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	handler := newOAuthHandler(server, m.tokens, http.DefaultClient, &m.oauthGuard)
	handler.mode = oauthModeInteractive
	handler.redirectURL = redirectURL
	handler.broker = m.broker
	handler.fetch = fetch

	sessionCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := m.authorizeServer(sessionCtx, server, handler); err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	if err := m.runAuthorizationHook(sessionCtx, server.ID, onAuthorized); err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}

	status := m.RefreshServer(sessionCtx, server.ID)
	if status.Status == "" {
		return mcpconfig.ServerStatus{Status: "error", Error: "The signed-in server is no longer available", CheckedAt: time.Now().UTC()}
	}
	return status
}

func (m *Manager) authorizeServer(ctx context.Context, server mcpconfig.Server, handler *oauthHandler) error {
	if ss := m.session(server.ID); ss != nil && ss.oauth != nil {
		previous := ss.oauth
		if previous.serverURL == handler.serverURL && previous.oauth == handler.oauth && previous.tokenID == handler.tokenID {
			if challenges, required := previous.authorizationChallenge(); required {
				return handler.authorizeChallenge(ctx, challenges)
			}
		}
	}
	ss, err := m.connect(ctx, server, handler)
	if err != nil {
		return err
	}
	closeSession(ss)
	if handler.didAuthorize() {
		return nil
	}
	return handler.AuthorizeFromMetadata(ctx)
}

func (m *Manager) runAuthorizationHook(ctx context.Context, serverID string, onAuthorized AuthorizationHook) error {
	if onAuthorized == nil {
		return nil
	}
	if err := onAuthorized(ctx); err != nil {
		tokenID := mcpconfig.OAuthConnectionID(serverID)
		unlock := integrationoauth.LockToken(tokenID)
		defer unlock()
		if deleteErr := m.tokens.DeleteToken(ctx, tokenID); deleteErr != nil {
			return fmt.Errorf("%w; rollback token: %v", err, deleteErr)
		}
		return err
	}
	return nil
}

func (m *Manager) OAuthRedirectURL(localCallback string) string {
	return m.broker.RedirectURL(localCallback)
}

type callbackReceiver struct {
	manager  *Manager
	started  chan<- string
	pending  *authorizationPending
	loopback net.Listener
}

type authorizationPending struct {
	result  chan mcpconfig.AuthorizationResponse
	done    chan struct{}
	status  mcpconfig.ServerStatus
	ctx     context.Context
	cancel  context.CancelFunc
	claimed atomic.Bool
}

func newAuthorizationPending() *authorizationPending {
	return &authorizationPending{
		result: make(chan mcpconfig.AuthorizationResponse, 1),
		done:   make(chan struct{}),
	}
}

func (p *authorizationPending) complete(status mcpconfig.ServerStatus) {
	p.status = status
	close(p.done)
}

func (p *authorizationPending) claimCallback() bool {
	return p.claimed.CompareAndSwap(false, true)
}

func (r *callbackReceiver) authorize(ctx context.Context, server mcpconfig.Server, redirectURL string, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	if server.OAuth.UsesLoopback() {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return mcpconfig.ServerStatus{Status: "error", Error: fmt.Sprintf("listen for local OAuth callback: %v", err), CheckedAt: time.Now().UTC()}
		}
		defer listener.Close()
		r.loopback = listener
		redirectURL = "http://" + listener.Addr().String() + mcpconfig.OAuthCallbackPath
	}
	return r.manager.runAuthorize(ctx, server, redirectURL, r.fetch, onAuthorized)
}

func (r *callbackReceiver) fetch(ctx context.Context, authURL string) (mcpconfig.AuthorizationResponse, error) {
	state := authorizationStateFromURL(authURL)
	if state == "" {
		return mcpconfig.AuthorizationResponse{}, errors.New("authorization URL did not include state")
	}
	if r.loopback != nil {
		r.serveLoopback(ctx, state)
	} else {
		r.pending.ctx = ctx
		if err := r.manager.registerAuthorizationState(state, r.pending); err != nil {
			return mcpconfig.AuthorizationResponse{}, err
		}
		defer r.manager.unregisterAuthorizationState(state, r.pending)
	}

	select {
	case r.started <- authURL:
	default:
	}
	select {
	case res := <-r.pending.result:
		return res, nil
	case <-ctx.Done():
		return mcpconfig.AuthorizationResponse{}, ctx.Err()
	}
}

func (m *Manager) CompleteAuthorization(ctx context.Context, response mcpconfig.AuthorizationResponse) error {
	if response.State == "" {
		return errors.New("authorization state is required")
	}
	pending, ok := m.takeAuthorizationState(response.State)
	if !ok {
		return errors.New("authorization state expired or was not started by Jaz")
	}
	return pending.accept(ctx, response)
}

func (p *authorizationPending) accept(ctx context.Context, response mcpconfig.AuthorizationResponse) error {
	// The callback claim allows one response, even if the browser closes before exchange completes.
	p.result <- response
	select {
	case <-p.done:
		status := p.status
		if status.Status == "connected" {
			return nil
		}
		if status.Error != "" {
			return errors.New(status.Error)
		}
		return fmt.Errorf("authorization finished with status %s", status.Status)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) registerAuthorizationState(state string, pending *authorizationPending) error {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if _, exists := m.authStates[state]; exists {
		return errors.New("authorization state is already pending")
	}
	m.authStates[state] = pending
	return nil
}

func (m *Manager) unregisterAuthorizationState(state string, pending *authorizationPending) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if m.authStates[state] == pending {
		delete(m.authStates, state)
	}
}

func (m *Manager) takeAuthorizationState(state string) (*authorizationPending, bool) {
	m.authMu.Lock()
	defer m.authMu.Unlock()
	pending, ok := m.authStates[state]
	if ok {
		delete(m.authStates, state)
		if pending.ctx.Err() != nil || !pending.claimCallback() {
			return nil, false
		}
	}
	return pending, ok
}
