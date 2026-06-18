package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

type UtilityPromptRequest struct {
	ACPAgent        string
	Directory       string
	Message         string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	Timeout         time.Duration
}

func (m *Manager) RunUtilityPrompt(ctx context.Context, req UtilityPromptRequest) (string, error) {
	if strings.TrimSpace(req.Message) == "" {
		return "", fmt.Errorf("message is required")
	}
	spawnReq, cfg, _, err := m.spawnConfig(SpawnRequest{
		ACPAgent:        req.ACPAgent,
		Directory:       req.Directory,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
	})
	if err != nil {
		return "", err
	}
	if req.Timeout <= 0 {
		req.Timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	cwd, _, err := m.prepareSessionDir(callCtx, spawnReq, cfg, "utility")
	if err != nil {
		return "", err
	}
	if cfg.Local {
		return m.runLocalUtilityPrompt(callCtx, spawnReq, cfg, cwd, req.Message)
	}
	collector := &utilityPromptCollector{}
	ac, err := m.connectWithHandler(callCtx, spawnReq.ACPAgent, cfg, cwd, "", jsonrpc.HandlerFunc(collector.handleJSONRPC))
	if err != nil {
		return "", err
	}
	defer ac.close()

	acpSession, err := m.newUtilitySession(callCtx, ac, spawnReq.ACPAgent, cfg, cwd)
	if err != nil {
		return "", err
	}
	collector.setSession(string(acpSession.response.SessionID))
	defer m.closeUtilitySession(ac, acpSession.response.SessionID)

	prompt, err := promptContentBlocks("", req.Message, nil)
	if err != nil {
		return "", err
	}
	raw, err := ac.peer.Call(callCtx, acpschema.AgentMethodSessionPrompt, map[string]any{
		"sessionId": acpSession.response.SessionID,
		"prompt":    prompt,
	})
	if err != nil {
		return "", ac.withProcessStderr(err)
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.StopReason == "cancelled" {
		return "", fmt.Errorf("utility prompt cancelled")
	}
	if text := strings.TrimSpace(collector.text()); text != "" {
		return text, nil
	}
	return "", fmt.Errorf("empty utility prompt response")
}

func (m *Manager) newUtilitySession(ctx context.Context, ac *agentConn, agent string, cfg AgentConfig, cwd string) (acpSessionInfo, error) {
	session, err := m.newACPProtocolSession(ctx, ac, "utility", newSessionRequest{
		Cwd:        cwd,
		MCPServers: []json.RawMessage{},
	})
	if err != nil {
		return acpSessionInfo{}, err
	}
	if _, err := m.configuredModeState(ctx, ac.peer, agent, session, cfg); err != nil {
		return acpSessionInfo{}, err
	}
	return session, nil
}

func (m *Manager) closeUtilitySession(ac *agentConn, sessionID acpschema.SessionID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = ac.peer.Call(ctx, acpschema.AgentMethodSessionClose, acpschema.CloseSessionRequest{
		SessionID: sessionID,
	})
}

type utilityPromptCollector struct {
	mu        sync.Mutex
	sessionID string
	assistant strings.Builder
	title     string
}

func (c *utilityPromptCollector) setSession(sessionID string) {
	c.mu.Lock()
	c.sessionID = sessionID
	c.mu.Unlock()
}

func (c *utilityPromptCollector) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if text := strings.TrimSpace(c.assistant.String()); text != "" {
		return text
	}
	return strings.TrimSpace(c.title)
}

func (c *utilityPromptCollector) handleJSONRPC(_ context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
	switch req.Method {
	case acpschema.ClientMethodSessionUpdate:
		return c.handleSessionUpdate(req.Params)
	case acpschema.ClientMethodSessionRequestPermission:
		return jsonrpc.EncodeResult(acpschema.RequestPermissionResponseCancelled())
	default:
		return nil, jsonrpc.MethodNotFound(req.Method)
	}
}

func (c *utilityPromptCollector) handleSessionUpdate(params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var note struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(params, &note); err != nil {
		return nil, jsonrpc.InvalidParams("invalid session/update", map[string]any{"error": err.Error()})
	}
	c.mu.Lock()
	sessionID := c.sessionID
	c.mu.Unlock()
	if sessionID != "" && note.SessionID != sessionID {
		return jsonrpc.EncodeResult(map[string]any{})
	}
	update, err := acpschema.DecodeSessionUpdate(note.Update)
	if err != nil {
		return nil, jsonrpc.InvalidParams("invalid session update payload", map[string]any{"error": err.Error()})
	}
	switch event := update.(type) {
	case acpschema.AgentMessageChunkUpdate:
		c.mu.Lock()
		c.assistant.WriteString(contentText(event.Content))
		c.mu.Unlock()
	case acpschema.SessionInfoSessionUpdate:
		c.mu.Lock()
		c.title = strings.TrimSpace(event.Title)
		c.mu.Unlock()
	}
	return jsonrpc.EncodeResult(map[string]any{})
}
