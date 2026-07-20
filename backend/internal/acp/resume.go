package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
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
	if session.RuntimeRef.SessionID == "" {
		hasTranscript, err := m.store.HasSessionTranscript(session.ID)
		if err != nil {
			return nil, fmt.Errorf("inspect ACP session transcript: %w", err)
		}
		if hasTranscript {
			return nil, fmt.Errorf("cannot safely resume ACP session %s: provider session id is missing for a thread with transcript history", session.ID)
		}
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
	acpSessionID, modes, loaded, err := m.restoreACPSession(ctx, ac, agentName, session, cfg, cwd, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		ac.close()
		return nil, err
	}
	storedACP := session.RuntimeRef.SessionID
	materializesOnPrompt := !loaded && sessionMaterializesOnPrompt(agentName, cfg)
	if materializesOnPrompt && storedACP != "" {
		session.RuntimeRef.SessionID = ""
		sessionChanged = true
	} else if !loaded && !materializesOnPrompt {
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
	var persistSessionID func()
	if materializesOnPrompt {
		persistSessionID = func() {
			updated, err := m.store.ReplaceRuntimeSessionID(session.ID, "", acpSessionID)
			if err != nil {
				m.log.Error("persist materialized agent session", "agent", agentName, "session", session.ID, "error", err)
			} else if !updated {
				m.log.Error("materialized agent session changed before persistence", "agent", agentName, "session", session.ID)
			}
		}
	}
	ac.trackPromptSends(job, persistSessionID)
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
func (m *Manager) restoreACPSession(ctx context.Context, ac *agentConn, agentName string, session storage.Session, cfg AgentConfig, cwd, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (string, ModeState, bool, error) {
	agentName = CanonicalAgentName(agentName)
	storedID := session.RuntimeRef.SessionID
	if loadSessionSupported(ac.initRaw) && storedID != "" {
		meta, err := m.sessionMeta(ctx, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
		if err != nil {
			return "", ModeState{}, false, err
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
				return "", ModeState{}, false, err
			}
			modes, err := m.configuredModeState(ctx, ac.peer, agentName, newACPSessionInfo(raw, acpschema.NewSessionResponse{
				SessionID: acpschema.SessionID(storedID),
				Modes:     resp.Modes,
			}), cfg)
			return storedID, modes, true, err
		}
		if turnScopedAgentProcess(cfg) {
			replaceable, inspectErr := m.replaceableUnmaterializedSession(session, agentName, cfg, err)
			if inspectErr != nil {
				return "", ModeState{}, false, inspectErr
			}
			if !replaceable {
				return "", ModeState{}, false, fmt.Errorf("resume ACP session %s: %w", storedID, err)
			}
			m.log.Warn("replacing unmaterialized agent session", "agent", agentName, "session", session.ID, "acp_session", storedID)
		}
	}
	acpSession, err := m.newACPSession(mcpsession.With(ctx, session.ID), ac, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return "", ModeState{}, false, err
	}
	modes, err := m.configuredModeState(ctx, ac.peer, agentName, acpSession, cfg)
	return string(acpSession.response.SessionID), modes, false, err
}

func (m *Manager) replaceableUnmaterializedSession(session storage.Session, agentName string, cfg AgentConfig, loadErr error) (bool, error) {
	var rpcErr *jsonrpc.Error
	if !sessionMaterializesOnPrompt(agentName, cfg) ||
		!errors.As(loadErr, &rpcErr) || rpcErr.Code != int(acpschema.ErrorCode32002) {
		return false, nil
	}
	hasTranscript, err := m.store.HasSessionTranscript(session.ID)
	if err != nil {
		return false, fmt.Errorf("inspect ACP session transcript: %w", err)
	}
	return !hasTranscript, nil
}
