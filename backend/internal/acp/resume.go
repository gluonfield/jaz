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
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/worktrees"
)

func (m *Manager) resume(ctx context.Context, ref string) (*jobState, error) {
	if job, err := m.job(ref); err == nil {
		return job, nil
	}
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return nil, fmt.Errorf("active acp session not found: %s", ref)
	}
	defer m.resumeLocks.Lock(session.ID)()
	return m.resumeLocked(ctx, session.ID, nil)
}

func (m *Manager) restart(ctx context.Context, stale *jobState) (*jobState, error) {
	defer m.resumeLocks.Lock(stale.ID)()
	m.mu.RLock()
	current, process := m.jobsByID[stale.ID], m.processes[stale.ID]
	m.mu.RUnlock()
	if current != nil && current != stale {
		return current, nil
	}
	var auth *cloudLaunchAuth
	if process != nil && process.cloudOAuth.version > 0 {
		oauth := process.cloudOAuth
		var rejectedVersion int64
		if stale.claudeAuthFailed() {
			rejectedVersion = oauth.version
		}
		var err error
		auth, err = m.cloudLaunchAuth(sessioncontext.WithWorkspaceID(ctx, oauth.workspaceID), oauth.profileID, rejectedVersion)
		if err != nil {
			return nil, err
		}
	}
	m.teardown(stale.ID)
	return m.resumeLocked(ctx, stale.ID, auth)
}

func (m *Manager) resumeLocked(ctx context.Context, ref string, auth *cloudLaunchAuth) (_ *jobState, err error) {
	ctx, finish, err := m.beginLaunch(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil && m.isClosed() {
			err = ErrClosed
		}
		finish()
	}()
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
	if err := worktrees.Ensure(ctx, m.store, m.cfg.Workspace, session); err != nil {
		return nil, err
	}
	ctx = mcpsession.With(ctx, session.ID)
	ctx = sessioncontext.WithSourceType(ctx, session.SourceType)
	ctx = sessioncontext.WithWorkspaceID(ctx, session.WorkspaceID)
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
	cfg, err := m.sessionAgentConfig(agentName, session.RuntimeRef.CloudCredentialProfileID)
	if err != nil {
		return nil, err
	}
	overview, err := m.store.LoadSessionOverviewEvents(session.ID)
	if err != nil {
		return nil, fmt.Errorf("restore agent session state: %w", err)
	}
	cfg.Model = strings.TrimSpace(session.Model)
	cfg.ReasoningEffort = strings.TrimSpace(session.ReasoningEffort)
	restoreConfiguredChoices(&cfg, overview)
	if cfg.UsesModelProvider() && cfg.cloudCredentialProfileID == "" {
		cfg.ModelProvider = strings.TrimSpace(session.ModelProvider)
		cfg = cfg.NormalizeProviderModel(cfg.ModelProvider)
		if err := m.validateAgentModelProvider(agentName, cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Local {
		if sessionChanged {
			if err := m.store.SaveSession(session); err != nil {
				return nil, err
			}
		}
		return m.resumeLocalSession(session, agentName, cfg)
	}
	if session.RuntimeRef.SessionID == "" {
		hasTranscript, err := m.store.HasAgentTranscript(session.ID)
		if err != nil {
			return nil, fmt.Errorf("inspect ACP session transcript: %w", err)
		}
		if hasTranscript && (session.Turn == nil || session.Turn.Prompt == nil || session.Turn.Prompt.State != "") {
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
	if auth == nil || auth.initial.ID != cfg.cloudCredentialProfileID || auth.workspaceID != session.WorkspaceID {
		auth, err = m.cloudLaunchAuth(ctx, cfg.cloudCredentialProfileID, 0)
		if err != nil {
			return nil, err
		}
	}
	ac, err := m.connect(ctx, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions, auth)
	if err != nil {
		return nil, err
	}
	if err := validateProcessLifecycle(agentName, cfg, ac.initRaw); err != nil {
		ac.close()
		return nil, err
	}
	mcpRevision := m.mcpRevision()
	acpSessionID, modes, loaded, err := m.restoreACPSession(ctx, ac, agentName, session, cfg, cwd, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		ac.close()
		return nil, err
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
	providerID := acpSessionID
	if !loaded && sessionMaterializesOnPrompt(agentName, cfg) {
		providerID = ""
	}
	if storedID := session.RuntimeRef.SessionID; storedID != providerID {
		if updated, err := m.store.ReplaceRuntimeSessionID(session.ID, storedID, providerID); !updated {
			ac.close()
			return nil, errors.Join(err, fmt.Errorf("replace provider session %s of %s", storedID, session.ID))
		}
	}
	job := newIdleJob(session, agentName, acpSessionID, cwd, modes)
	job.backgroundTasks = make(map[string]sessionevents.AgentTask)
	for _, event := range overview {
		if event.AgentTask != nil {
			job.backgroundTasks[event.AgentTask.ID] = *event.AgentTask
		}
	}
	job.steerMethod = supportedSteerMethod(ac.initRaw)
	m.trackPromptSends(ac, job)
	process := newAgentProcess(ac)
	process.mcpPolicy = mcpServerPolicy
	process.mcpRevision = mcpRevision
	process.mcpRefresh = supportedMCPRefresh(ac.initRaw, mcpServerPolicy)
	m.disconnectAgentWork(job)
	if err := m.register(job, process); err != nil {
		ac.close()
		return nil, err
	}
	ac.state.attach(m, job, cfg)
	m.restoreSessionControls(ctx, job, overview)
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

// session/resume restores provider state without replaying history. The job is
// registered after the legacy session/load fallback so its replay is dropped.
func (m *Manager) restoreACPSession(ctx context.Context, ac *agentConn, agentName string, session storage.Session, cfg AgentConfig, cwd, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (string, ModeState, bool, error) {
	agentName = CanonicalAgentName(agentName)
	storedID := session.RuntimeRef.SessionID
	restoreMethod := sessionRestoreMethod(ac.initRaw)
	if storedID != "" {
		if restoreMethod == "" {
			return "", ModeState{}, false, fmt.Errorf("agent %q cannot restore stored session %s", agentName, storedID)
		}
		meta, err := m.sessionRestoreMeta(ctx, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, session.SourceType, systemPromptExtensions)
		if err != nil {
			return "", ModeState{}, false, err
		}
		raw, err := ac.peer.Call(ctx, restoreMethod, struct {
			Meta       map[string]any      `json:"_meta,omitempty"`
			Cwd        string              `json:"cwd"`
			MCPServers []json.RawMessage   `json:"mcpServers"`
			SessionID  acpschema.SessionID `json:"sessionId"`
		}{
			Meta:       meta,
			Cwd:        cwd,
			MCPServers: m.mcpServersForAgent(ctx, ac.initRaw, mcpServerPolicy),
			SessionID:  acpschema.SessionID(storedID),
		})
		if err == nil {
			var resp acpschema.LoadSessionResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return "", ModeState{}, false, err
			}
			ac.state.initial = raw
			modes, err := m.configuredModeState(ctx, ac, agentName, newACPSessionInfo(raw, acpschema.NewSessionResponse{
				SessionID: acpschema.SessionID(storedID),
				Modes:     resp.Modes,
			}), cfg)
			return storedID, modes, true, err
		}
		replaceable, inspectErr := m.replaceableMissingSession(session, agentName, cfg, err)
		if inspectErr != nil {
			return "", ModeState{}, false, inspectErr
		}
		if !replaceable {
			if isMissingACPSession(err) {
				return "", ModeState{}, false, fmt.Errorf("cannot resume ACP session %s: the agent no longer has its conversation history", storedID)
			}
			return "", ModeState{}, false, withSignInHint(fmt.Errorf("resume ACP session %s: %w", storedID, err))
		}
		m.log.Warn("replacing missing agent session without transcript", "agent", agentName, "session", session.ID, "acp_session", storedID)
	}
	acpSession, err := m.newACPSession(ctx, ac, agentName, cfg, cwd, session, systemPromptExtensions)
	if err != nil {
		return "", ModeState{}, false, err
	}
	modes, err := m.configuredModeState(ctx, ac, agentName, acpSession, cfg)
	return string(acpSession.response.SessionID), modes, false, err
}

func (m *Manager) replaceableMissingSession(session storage.Session, agentName string, cfg AgentConfig, loadErr error) (bool, error) {
	rpcErr, ok := errors.AsType[*jsonrpc.Error](loadErr)
	if !isMissingACPSession(loadErr) && (!ok || rpcErr.Code != int(acpschema.ErrorCode32002) || !sessionMaterializesOnPrompt(agentName, cfg)) {
		return false, nil
	}
	hasTranscript, err := m.store.HasAgentTranscript(session.ID)
	if err != nil {
		return false, fmt.Errorf("inspect ACP session transcript: %w", err)
	}
	return !hasTranscript, nil
}

func isMissingACPSession(err error) bool {
	rpcErr, ok := errors.AsType[*jsonrpc.Error](err)
	if !ok || rpcErr.Code != int(acpschema.ErrorCode32602) {
		return false
	}
	message, _, _ := strings.Cut(rpcErr.Message, ":")
	return strings.EqualFold(message, "Session not found")
}
