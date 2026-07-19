package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) resume(ctx context.Context, ref string) (*jobState, error) {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	return m.resumeLocked(ctx, ref)
}

func (m *Manager) restart(ctx context.Context, stale *jobState) (*jobState, error) {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
	if current := m.jobByID(stale.ID); current != nil && current != stale {
		return current, nil
	}
	m.teardown(stale.ID)
	return m.resumeLocked(ctx, stale.ID)
}

func (m *Manager) resumeLocked(ctx context.Context, ref string) (*jobState, error) {
	if job, err := m.job(ref); err == nil {
		return job, nil
	}
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return nil, fmt.Errorf("active acp session not found: %s", ref)
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil || session.RuntimeRef.Agent == "" {
		return nil, fmt.Errorf("session %s is not acp-backed", ref)
	}
	mcpServerPolicy := effectiveMCPServerPolicy(session)
	agentName := CanonicalAgentName(session.RuntimeRef.Agent)
	sessionChanged := false
	if agentName != session.RuntimeRef.Agent {
		if session.ModelProvider == session.RuntimeRef.Agent {
			session.ModelProvider = agentName
		}
		session.RuntimeRef.Agent = agentName
		sessionChanged = true
	}
	if mcpServerPolicy != "" && session.RuntimeRef.MCPServerPolicy == "" {
		session.RuntimeRef.MCPServerPolicy = mcpServerPolicy
		sessionChanged = true
	}
	cfg, ok, err := m.configuredAgent(agentName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("acp agent %q is not configured", agentName)
	}
	cfg.Model = strings.TrimSpace(session.Model)
	if cfg.UsesModelProvider() {
		cfg.ModelProvider = strings.TrimSpace(session.ModelProvider)
		cfg = cfg.NormalizeProviderModel(cfg.ModelProvider)
	}
	cfg.ReasoningEffort = strings.TrimSpace(session.ReasoningEffort)
	if cfg.Local {
		if sessionChanged {
			if err := m.store.SaveSession(session); err != nil {
				return nil, err
			}
		}
		return m.resumeLocalSession(session, agentName, cfg)
	}
	cwd := session.RuntimeRef.Cwd
	if cwd == "" {
		if cwd, err = m.resolveCwd(cfg.Cwd); err != nil {
			return nil, err
		}
	}
	systemPromptExtensions, err := m.resumeSystemPromptExtensions(session)
	if err != nil {
		return nil, err
	}
	ac, err := m.connect(ctx, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return nil, err
	}
	if err := validateProcessLifecycle(agentName, cfg, ac.initRaw); err != nil {
		ac.close()
		return nil, err
	}
	acpSessionID, modes, err := m.restoreACPSession(ctx, ac, agentName, session, cfg, cwd, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		ac.close()
		return nil, err
	}
	loaded := acpSessionID == session.RuntimeRef.SessionID
	if !loaded {
		session.RuntimeRef.SessionID = acpSessionID
		sessionChanged = true
	}
	if session.ModelProvider == "" {
		session.ModelProvider = session.RuntimeRef.Agent
		sessionChanged = true
	}
	if sessionChanged {
		if err := m.store.SaveSession(session); err != nil {
			ac.close()
			return nil, err
		}
	}
	job := newIdleJob(session, agentName, acpSessionID, cwd, modes)
	job.promptQueueing = promptQueueingSupported(ac.initRaw)
	ac.trackPromptSends(job)
	m.addJob(job, newAgentProcess(ac, turnScopedAgentProcess(cfg)))
	m.log.Info("resumed agent session", "agent", job.ACPAgent, "session", job.ID,
		"acp_session", acpSessionID, "loaded", loaded)
	return job, nil
}

func (m *Manager) resumeSystemPromptExtensions(session storage.Session) (promptmodule.Modules, error) {
	if m.cfg.ResumePrompt == nil {
		return nil, nil
	}
	extensions, err := m.cfg.ResumePrompt(session)
	if err != nil {
		return nil, err
	}
	return promptmodule.New(extensions...), nil
}

// The job is registered only after session/load returns, so the agent's
// history replay notifications are dropped, not re-recorded as events.
func (m *Manager) restoreACPSession(ctx context.Context, ac *agentConn, agentName string, session storage.Session, cfg AgentConfig, cwd, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (string, ModeState, error) {
	agentName = CanonicalAgentName(agentName)
	storedID := session.RuntimeRef.SessionID
	if loadSessionSupported(ac.initRaw) && storedID != "" {
		meta, err := m.sessionMeta(ctx, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
		if err != nil {
			return "", ModeState{}, err
		}
		mcpCtx := mcpsession.With(ctx, session.ID)
		raw, err := ac.peer.Call(mcpCtx, acpschema.AgentMethodSessionLoad, struct {
			Meta       map[string]any      `json:"_meta,omitempty"`
			Cwd        string              `json:"cwd"`
			MCPServers []json.RawMessage   `json:"mcpServers"`
			SessionID  acpschema.SessionID `json:"sessionId"`
		}{
			Meta:       meta,
			Cwd:        cwd,
			MCPServers: m.mcpServersForAgent(mcpCtx, ac.initRaw, mcpServerPolicy),
			SessionID:  acpschema.SessionID(storedID),
		})
		if err == nil {
			var resp acpschema.LoadSessionResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return "", ModeState{}, err
			}
			modes, err := m.configuredModeState(ctx, ac.peer, agentName, newACPSessionInfo(raw, acpschema.NewSessionResponse{
				SessionID: acpschema.SessionID(storedID),
				Modes:     resp.Modes,
			}), cfg)
			return storedID, modes, err
		}
		if turnScopedAgentProcess(cfg) {
			return "", ModeState{}, fmt.Errorf("resume ACP session %s: %w", storedID, err)
		}
	}
	acpSession, err := m.newACPSession(mcpsession.With(ctx, session.ID), ac, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return "", ModeState{}, err
	}
	modes, err := m.configuredModeState(ctx, ac.peer, agentName, acpSession, cfg)
	return string(acpSession.response.SessionID), modes, err
}
