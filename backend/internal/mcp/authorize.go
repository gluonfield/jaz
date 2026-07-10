package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

type AuthorizationHook = func(context.Context) error

// Authorize runs the interactive OAuth authorization-code flow for a server,
// persists the resulting token, and reconnects so the server's tools become
// available. Browser clients receive the provider URL immediately and finish
// through the shared callback route; desktop callers keep the historical blocking
// loopback flow.
func (m *Manager) Authorize(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions) mcpconfig.ServerStatus {
	return m.authorize(ctx, server, opts, nil)
}

func (m *Manager) AuthorizeWithHook(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	return m.authorize(ctx, server, opts, onAuthorized)
}

func (m *Manager) authorize(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	if m.tokens == nil {
		return mcpconfig.ServerStatus{Status: "error", Error: "token store is not configured", CheckedAt: time.Now().UTC()}
	}
	if opts.ReturnAuthURL {
		return m.authorizeWithCallback(ctx, server, opts, onAuthorized)
	}
	receiver, err := newLoopbackReceiver()
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	defer receiver.close()

	return m.runAuthorize(ctx, server, receiver.redirectURL, receiver.fetch, onAuthorized)
}

func (m *Manager) authorizeWithCallback(ctx context.Context, server mcpconfig.Server, opts mcpconfig.AuthorizeOptions, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	if strings.TrimSpace(opts.RedirectURL) == "" {
		return mcpconfig.ServerStatus{Status: "error", Error: "OAuth redirect URL is not configured", CheckedAt: time.Now().UTC()}
	}
	started := make(chan authorizationStart, 1)
	pending := newAuthorizationPending()
	receiver := &callbackReceiver{
		manager:     m,
		redirectURL: strings.TrimSpace(opts.RedirectURL),
		started:     started,
		openBrowser: opts.OpenBrowser,
		pending:     pending,
	}
	flowCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		status := m.runAuthorize(flowCtx, server, receiver.redirectURL, receiver.fetch, onAuthorized)
		m.setServerStatus(server.ID, status)
		pending.complete(status)
	}()

	select {
	case start := <-started:
		return mcpconfig.ServerStatus{
			Status:    "needs_auth",
			Error:     "Sign in required",
			AuthURL:   start.authURL,
			CheckedAt: time.Now().UTC(),
		}
	case status := <-pending.done:
		return status
	case <-ctx.Done():
		return mcpconfig.ServerStatus{Status: "error", Error: ctx.Err().Error(), CheckedAt: time.Now().UTC()}
	}
}

func (m *Manager) runAuthorize(ctx context.Context, server mcpconfig.Server, redirectURL string, fetch codeFetcher, onAuthorized AuthorizationHook) mcpconfig.ServerStatus {
	handler := newOAuthHandler(server, m.tokens, http.DefaultClient)
	handler.mode = oauthModeInteractive
	handler.redirectURL = redirectURL
	handler.fetch = fetch

	sessionCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	ss, err := m.connect(sessionCtx, server, handler)
	if err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}
	closeSessions(map[string]*serverSession{server.ID: ss})
	if !handler.didAuthorize() {
		if err := handler.AuthorizeFromMetadata(sessionCtx); err != nil {
			return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
		}
	}
	if err := m.runAuthorizationHook(sessionCtx, server.ID, onAuthorized); err != nil {
		return mcpconfig.ServerStatus{Status: "error", Error: err.Error(), CheckedAt: time.Now().UTC()}
	}

	// Reconnect everything so this server's tools are installed in the registry
	// using the token we just persisted.
	m.Refresh(context.Background())
	if status := m.Status(server.ID); status.Status != "" {
		return status
	}
	return connectedStatus(ss.tools)
}

func (m *Manager) runAuthorizationHook(ctx context.Context, serverID string, onAuthorized AuthorizationHook) error {
	if onAuthorized == nil {
		return nil
	}
	if err := onAuthorized(ctx); err != nil {
		tokenID := mcpconfig.OAuthConnectionID(serverID)
		if deleteErr := m.tokens.DeleteToken(ctx, tokenID); deleteErr != nil {
			return fmt.Errorf("%w; rollback token: %v", err, deleteErr)
		}
		return err
	}
	return nil
}

type authorizationStart struct {
	authURL string
}

type callbackReceiver struct {
	manager     *Manager
	redirectURL string
	started     chan<- authorizationStart
	openBrowser bool
	pending     *authorizationPending
}

type authorizationPending struct {
	result chan loopbackResult
	done   chan mcpconfig.ServerStatus
}

func newAuthorizationPending() *authorizationPending {
	return &authorizationPending{
		result: make(chan loopbackResult, 1),
		done:   make(chan mcpconfig.ServerStatus, 1),
	}
}

func (p *authorizationPending) complete(status mcpconfig.ServerStatus) {
	select {
	case p.done <- status:
	default:
	}
}

func (r *callbackReceiver) fetch(ctx context.Context, authURL string) (string, string, error) {
	state := authorizationStateFromURL(authURL)
	if state == "" {
		return "", "", errors.New("authorization URL did not include state")
	}
	if err := r.manager.registerAuthorizationState(state, r.pending); err != nil {
		return "", "", err
	}
	defer r.manager.unregisterAuthorizationState(state, r.pending)

	if r.openBrowser {
		if err := openBrowser(authURL); err != nil {
			fmt.Printf("Open this URL to authorize the MCP server:\n%s\n", authURL)
		}
	}
	select {
	case r.started <- authorizationStart{authURL: authURL}:
	default:
	}
	select {
	case res := <-r.pending.result:
		if res.err != "" {
			return "", "", fmt.Errorf("authorization failed: %s", res.err)
		}
		if res.code == "" {
			return "", "", errors.New("authorization returned no code")
		}
		return res.code, res.state, nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

func (m *Manager) CompleteAuthorization(ctx context.Context, state, code, failure string) error {
	state = strings.TrimSpace(state)
	if state == "" {
		return errors.New("authorization state is required")
	}
	pending, ok := m.takeAuthorizationState(state)
	if !ok {
		return errors.New("authorization state expired or was not started by Jaz")
	}
	select {
	case pending.result <- loopbackResult{code: strings.TrimSpace(code), state: state, err: strings.TrimSpace(failure)}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case status := <-pending.done:
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
	if m.authStates == nil {
		m.authStates = make(map[string]*authorizationPending)
	}
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
	}
	return pending, ok
}
