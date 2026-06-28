package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	StateStarting   = "starting"
	StateRunning    = "running"
	StateIdle       = "idle"
	StateFailed     = "failed"
	StateCancelled  = "cancelled"
	StateNotRunning = "not_running"
)

const (
	MCPServerPolicyAll                = ""
	MCPServerPolicyWidget             = "widget"
	MCPServerPolicyMemorySearchWorker = "memory_search_worker"
	MCPServerPolicyBrowserWorker      = "browser_worker"
)

type Store interface {
	CreateSession(storage.CreateSession) (storage.Session, error)
	LoadSession(string) (storage.Session, error)
	SaveSession(storage.Session) error
	TouchSessionAttention(string) error
	storage.MessageAppender
	storage.SessionEventAppender
	storage.ActivityUpserter
}

type Manager struct {
	cfg          Config
	agents       AgentConfigSource
	store        Store
	log          *log.Logger
	Done         func(context.Context, Job)
	TurnFinished func(context.Context, Job)

	Events *sessionevents.Bus
	// PublishWidget backs the _jaz.dev/widget/publish extension method; the
	// session id is the jaz session linked to the calling agent's ACP session.
	PublishWidget func(WidgetPublishRequest) (WidgetPublishResult, error)

	mu           sync.RWMutex
	jobsByID     map[string]*jobState
	jobsBySlug   map[string]*jobState
	jobsByACP    map[string]*jobState
	connsByID    map[string]jsonrpc.MessageConn
	peersByID    map[string]*jsonrpc.Peer
	cancelByID   map[string]context.CancelFunc
	serveErrByID map[string]error
	localAgents  map[string]LocalAgentRunner

	permissionSeq     uint64
	pendingPermission map[string]*pendingPermission
	permissionMu      sync.Mutex

	transcriptBuffers acpTranscriptBuffers

	// serializes resumes so concurrent sends can't start two agent processes
	resumeMu sync.Mutex
}

type SpawnRequest struct {
	ParentID string
	ACPAgent string
	Slug     string
	Title    string
	// Directory is where the agent works, relative to the jaz workspace
	// (absolute paths must stay inside it); created if missing. Empty means
	// a fresh per-session directory named after the slug.
	Directory string
	// Worktree runs the session on a disposable git worktree of Directory.
	Worktree bool
	// Branch selects the base branch/ref for Worktree. Empty means Directory's HEAD.
	Branch string
	// Model overrides the agent's configured model for this session (empty
	// keeps the agent default).
	Model string
	// ModelProvider selects the provider for in-process agents such as Jaz.
	ModelProvider string
	// ReasoningEffort overrides the agent's configured reasoning effort for this
	// session (empty keeps the agent default).
	ReasoningEffort string
	// SourceType/SourceID tag sessions created by backend automation such as loops.
	SourceType      string
	SourceID        string
	ArtifactSurface string
	MCPServerPolicy string
	// SystemPromptExtensions are runtime-only prompt modules appended at
	// session creation. They are not persisted in thread storage.
	SystemPromptExtensions promptmodule.Modules
}

type SendRequest struct {
	Session       string
	Message       string
	Contexts      []storage.MessageContext
	Attachments   []storage.Attachment
	Completion    CompletionMode
	PlanRequested bool
	GoalRequested bool
	ParentVisible bool
}

type CompactRequest struct {
	Session string
}

const (
	ActiveOperationCompact = "compact"
	CompactCommand         = "/compact"
)

type SteerRequest struct {
	Session       string
	Message       string
	Contexts      []storage.MessageContext
	Attachments   []storage.Attachment
	GoalRequested bool
	ParentVisible bool
}

type InteractiveAnswer struct {
	Session       string
	RequestID     string
	OptionID      string
	Text          string
	Answers       map[string]InteractiveAnswerValue
	PlanRequested bool
	ParentVisible bool
}

type WaitRequest struct {
	Session string
	Timeout time.Duration
}

type SpawnResult struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
	Slug      string `json:"slug"`
	ACPAgent  string `json:"acp_agent"`
	Cwd       string `json:"cwd,omitempty"`
	State     string `json:"state"`
	// Session is the persisted row, handed to in-process callers (the HTTP
	// create handler) so they don't re-read what Spawn just wrote. Excluded from
	// the tool-facing JSON above.
	Session storage.Session `json:"-"`
}

func NewManager(store Store, cfg Config, logger *log.Logger) *Manager {
	agents := cfg.AgentSource
	if agents == nil {
		agents = MergeAgents(nil, cfg.Agents)
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{
		cfg:               cfg,
		agents:            agents,
		store:             store,
		log:               logger.WithPrefix("acp"),
		jobsByID:          make(map[string]*jobState),
		jobsBySlug:        make(map[string]*jobState),
		jobsByACP:         make(map[string]*jobState),
		connsByID:         make(map[string]jsonrpc.MessageConn),
		peersByID:         make(map[string]*jsonrpc.Peer),
		cancelByID:        make(map[string]context.CancelFunc),
		serveErrByID:      make(map[string]error),
		localAgents:       make(map[string]LocalAgentRunner),
		pendingPermission: make(map[string]*pendingPermission),
	}
}

// providers returns the current effective provider set. It reads the live
// ProviderSource (so a runtime add/edit/delete reaches the next spawn) when one
// is configured, falling back to the static Providers snapshot used by tests and
// the read-time auth/readiness probes.
func (m *Manager) providers() map[string]provider.ModelProviderConfig {
	if m.cfg.ProviderSource != nil {
		return m.cfg.ProviderSource.Providers()
	}
	return m.cfg.Providers
}

type agentConn struct {
	conn          jsonrpc.MessageConn
	peer          *jsonrpc.Peer
	cancel        context.CancelFunc
	initRaw       json.RawMessage
	stderr        *processStderrTail
	promptTracker *promptTrackingConn
}

func (c *agentConn) close() {
	_ = c.peer.Close()
	c.cancel()
}

func (c *agentConn) withProcessStderr(err error) error {
	return withProcessStderr(err, c.stderr)
}

func (c *agentConn) trackPromptSends(job *jobState) {
	if c.promptTracker != nil {
		c.promptTracker.setOnPromptSent(job.markFirstPromptSent)
	}
}

func (m *Manager) connect(ctx context.Context, name string, cfg AgentConfig, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (*agentConn, error) {
	return m.connectWithHandler(ctx, name, cfg, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions, jsonrpc.HandlerFunc(m.handleJSONRPC))
}

func (m *Manager) connectWithHandler(ctx context.Context, name string, cfg AgentConfig, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules, handler jsonrpc.Handler) (*agentConn, error) {
	env, err := m.processEnvPreparedForSurfacePolicy(ctx, name, cfg, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return nil, err
	}
	cfg = configForMCPServerPolicy(name, cfg, mcpServerPolicy)
	runCtx, cancel := context.WithCancel(context.Background())
	conn, stderr, err := m.openConn(runCtx, name, cfg, env, cwd)
	if err != nil {
		cancel()
		return nil, err
	}
	promptTracker := newPromptTrackingConn(conn)
	peer := jsonrpc.NewPeer(promptTracker, handler)
	go func() {
		err := peer.Serve(runCtx)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			m.setServeErr(peer, err)
		}
	}()

	initRaw, err := peer.Call(ctx, acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersion(acpschema.ProtocolVersionNumber),
		ClientInfo: &acpschema.Implementation{
			Name:    "jaz",
			Title:   "Jaz",
			Version: "0.1.0",
		},
		ClientCapabilities: &acpschema.ClientCapabilities{
			Meta: map[string]any{
				"terminal-auth":  true,
				"jaz.dev/widget": map[string]any{"version": 1},
			},
			FS: &acpschema.FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Elicitation: &acpschema.ElicitationCapabilities{Form: &acpschema.ElicitationFormCapabilities{}},
		},
	})
	if err != nil {
		_ = peer.Close()
		cancel()
		return nil, fmt.Errorf("initialize acp agent: %w", withProcessStderr(err, stderr))
	}
	methodID, missingAuth := autoAuthMethod(name, initRaw, env)
	if methodID != "" {
		if _, err := peer.Call(ctx, acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID}); err != nil {
			_ = peer.Close()
			cancel()
			return nil, fmt.Errorf("authenticate acp agent: %w", withProcessStderr(err, stderr))
		}
	} else if len(missingAuth) > 0 {
		_ = peer.Close()
		cancel()
		return nil, fmt.Errorf("authenticate acp agent %q: missing %s", name, strings.Join(missingAuth, " or "))
	}
	return &agentConn{conn: promptTracker, peer: peer, cancel: cancel, initRaw: initRaw, stderr: stderr, promptTracker: promptTracker}, nil
}

// sessionMeta builds the session _meta payload for prompt and agent-specific
// options.
func (m *Manager) sessionMeta(ctx context.Context, agent string, cfg AgentConfig, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (map[string]any, error) {
	meta, err := m.sessionPromptMeta(ctx, agent, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return nil, err
	}
	return agentPolicyForAgent(agent).mergeSessionMeta(meta, cfg.ReasoningEffort), nil
}

func (m *Manager) sessionPromptMeta(ctx context.Context, agent, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (map[string]any, error) {
	prompt, err := m.systemPrompt(ctx, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return nil, err
	}
	if prompt == "" {
		return nil, nil
	}
	return systemPromptMeta(agent, prompt), nil
}

type newSessionRequest struct {
	Meta       map[string]any    `json:"_meta,omitempty"`
	Cwd        string            `json:"cwd"`
	MCPServers []json.RawMessage `json:"mcpServers"`
}

func (m *Manager) newACPProtocolSession(ctx context.Context, ac *agentConn, label string, req newSessionRequest) (acpSessionInfo, error) {
	sessionRaw, err := ac.peer.Call(ctx, acpschema.AgentMethodSessionNew, req)
	if err != nil {
		return acpSessionInfo{}, fmt.Errorf("create acp %s session: %w", label, ac.withProcessStderr(err))
	}
	var acpSession acpschema.NewSessionResponse
	if err := json.Unmarshal(sessionRaw, &acpSession); err != nil {
		return acpSessionInfo{}, err
	}
	if acpSession.SessionID == "" {
		return acpSessionInfo{}, fmt.Errorf("acp session/new returned empty session id")
	}
	return newACPSessionInfo(sessionRaw, acpSession), nil
}

func (m *Manager) newACPSession(ctx context.Context, ac *agentConn, agent string, cfg AgentConfig, cwd, artifactSurface, mcpServerPolicy string, systemPromptExtensions promptmodule.Modules) (acpSessionInfo, error) {
	meta, err := m.sessionMeta(ctx, agent, cfg, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return acpSessionInfo{}, err
	}
	return m.newACPProtocolSession(ctx, ac, "agent", newSessionRequest{
		Meta:       meta,
		Cwd:        cwd,
		MCPServers: m.mcpServersForAgent(ctx, ac.initRaw, mcpServerPolicy),
	})
}

type createdSession struct {
	Request SpawnRequest
	Config  AgentConfig
	Session storage.Session
}

func (m *Manager) CreateSession(ctx context.Context, req SpawnRequest) (storage.Session, error) {
	created, err := m.createSession(ctx, req)
	if err != nil {
		return storage.Session{}, err
	}
	return created.Session, nil
}

func (m *Manager) createSession(ctx context.Context, req SpawnRequest) (createdSession, error) {
	req, cfg, effort, err := m.spawnConfig(req)
	if err != nil {
		return createdSession{}, err
	}
	session, err := m.createStoredSession(req, cfg, effort)
	if err != nil {
		return createdSession{}, err
	}
	fail := func(err error) (createdSession, error) {
		session.Status = storage.StatusError
		session.Error = err.Error()
		_ = m.store.SaveSession(session)
		return createdSession{}, err
	}
	absCwd, projectPath, err := m.prepareSessionDir(ctx, req, cfg, session.Slug)
	if err != nil {
		return fail(err)
	}
	session.RuntimeRef.Cwd = absCwd
	session.RuntimeRef.ProjectPath = projectPath
	if err := m.store.SaveSession(session); err != nil {
		return fail(err)
	}
	return createdSession{Request: req, Config: cfg, Session: session}, nil
}

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	created, err := m.createSession(ctx, req)
	if err != nil {
		return SpawnResult{}, err
	}
	req = created.Request
	cfg := created.Config
	session := created.Session
	fail := func(err error) (SpawnResult, error) {
		session.Status = storage.StatusError
		session.Error = err.Error()
		_ = m.store.SaveSession(session)
		return SpawnResult{}, err
	}
	absCwd := session.RuntimeRef.Cwd
	if cfg.Local {
		return m.spawnLocalSession(session, req.ACPAgent, absCwd, req.SystemPromptExtensions)
	}
	ac, err := m.connect(ctx, req.ACPAgent, cfg, absCwd, session.RuntimeRef.ArtifactSurface, session.RuntimeRef.MCPServerPolicy, req.SystemPromptExtensions)
	if err != nil {
		return fail(err)
	}
	acpSession, err := m.newACPSession(mcpsession.With(ctx, session.ID), ac, req.ACPAgent, cfg, absCwd, session.RuntimeRef.ArtifactSurface, session.RuntimeRef.MCPServerPolicy, req.SystemPromptExtensions)
	if err != nil {
		ac.close()
		return fail(err)
	}
	modes, err := m.configuredModeState(ctx, ac.peer, req.ACPAgent, acpSession, cfg)
	if err != nil {
		ac.close()
		return fail(err)
	}
	session.RuntimeRef.SessionID = string(acpSession.response.SessionID)
	session.RuntimeRef.Cwd = absCwd
	session.RuntimeRef.Capabilities = runtimeCapabilitiesFromInit(ac.initRaw)
	if err := m.store.SaveSession(session); err != nil {
		ac.close()
		return fail(err)
	}
	job := newIdleJob(session, req.ACPAgent, string(acpSession.response.SessionID), absCwd, modes)
	job.promptQueueing = promptQueueingSupported(ac.initRaw)
	job.nativeGoal = runtimeCapabilitiesNativeGoal(session.RuntimeRef.Capabilities)
	ac.trackPromptSends(job)
	m.addJob(job, ac.conn, ac.peer, ac.cancel)
	m.saveACPState(job.Snapshot())
	m.log.Info("spawned agent session", "agent", job.ACPAgent, "session", job.ID, "acp_session", job.ACPSession)

	return SpawnResult{
		Status:    "created",
		SessionID: job.ID,
		Slug:      job.Slug,
		ACPAgent:  job.ACPAgent,
		Cwd:       job.Cwd,
		State:     StateIdle,
		Session:   session,
	}, nil
}

func (m *Manager) initializeModeState(ctx context.Context, peer *jsonrpc.Peer, agentName string, session acpschema.NewSessionResponse) (ModeState, error) {
	acpModes := session.Modes
	modes := modeStateFromACP(acpModes)
	if acpModes == nil {
		return modes, nil
	}
	target := preferredBaselineModeID(agentName, acpModes.AvailableModes)
	if target == "" {
		target = baselineModeID(agentName, modes)
	}
	if target != "" && modes.CurrentModeID != target {
		if err := m.setSessionMode(ctx, peer, session.SessionID, target); err != nil {
			return modes, err
		}
		modes.CurrentModeID = target
	}
	return modes, nil
}

// Restarts the agent for a stored session (server restart): session/load when
// supported, otherwise a fresh agent session in the same workspace.
func (m *Manager) resume(ctx context.Context, ref string) (*jobState, error) {
	m.resumeMu.Lock()
	defer m.resumeMu.Unlock()
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
	var state storage.ACPState
	if loader, ok := m.store.(acpStateLoader); ok {
		state, _ = loader.LoadACPState(session.ID)
	}
	cwd := firstNonEmpty(session.RuntimeRef.Cwd, state.Cwd)
	if cwd == "" {
		if cwd, err = m.resolveCwd(cfg.Cwd); err != nil {
			return nil, err
		}
	}
	artifactSurface := ""
	if session.RuntimeRef != nil {
		artifactSurface = session.RuntimeRef.ArtifactSurface
	}
	systemPromptExtensions, err := m.resumeSystemPromptExtensions(session)
	if err != nil {
		return nil, err
	}
	ac, err := m.connect(ctx, agentName, cfg, cwd, artifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return nil, err
	}
	acpSessionID, modes, err := m.restoreACPSession(ctx, ac, agentName, session, cfg, cwd, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		ac.close()
		return nil, err
	}
	if acpSessionID != session.RuntimeRef.SessionID {
		session.RuntimeRef.SessionID = acpSessionID
		sessionChanged = true
	}
	runtimeCapabilities := runtimeCapabilitiesFromInit(ac.initRaw)
	if runtimeCapabilitiesNativeGoal(session.RuntimeRef.Capabilities) != runtimeCapabilitiesNativeGoal(runtimeCapabilities) {
		session.RuntimeRef.Capabilities = runtimeCapabilities
		sessionChanged = true
	}
	if session.ModelProvider == "" {
		session.ModelProvider = session.RuntimeRef.Agent
		sessionChanged = true
	}
	if sessionChanged {
		_ = m.store.SaveSession(session)
	}
	job := newIdleJob(session, agentName, acpSessionID, cwd, modes)
	job.ParentVisible = state.ParentVisible
	job.LastEventAt = firstNonZeroTime(state.LastEventAt, state.UpdatedAt)
	job.LastToolAt = state.LastToolAt
	job.promptQueueing = promptQueueingSupported(ac.initRaw)
	job.nativeGoal = runtimeCapabilitiesNativeGoal(session.RuntimeRef.Capabilities)
	ac.trackPromptSends(job)
	m.addJob(job, ac.conn, ac.peer, ac.cancel)
	m.saveACPState(job.Snapshot())
	m.log.Info("resumed agent session", "agent", job.ACPAgent, "session", job.ID,
		"acp_session", acpSessionID, "loaded", acpSessionID == session.RuntimeRef.SessionID)
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
	var caps struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	_ = json.Unmarshal(ac.initRaw, &caps)
	storedID := session.RuntimeRef.SessionID
	if caps.AgentCapabilities.LoadSession && storedID != "" {
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
		// The agent lost this session — fall through to a fresh one.
	}
	acpSession, err := m.newACPSession(mcpsession.With(ctx, session.ID), ac, agentName, cfg, cwd, session.RuntimeRef.ArtifactSurface, mcpServerPolicy, systemPromptExtensions)
	if err != nil {
		return "", ModeState{}, err
	}
	modes, err := m.configuredModeState(ctx, ac.peer, agentName, acpSession, cfg)
	return string(acpSession.response.SessionID), modes, err
}

func (m *Manager) Status(ref string) (Job, error) {
	if job, err := m.job(ref); err == nil {
		return job.Snapshot(), nil
	}
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return Job{}, err
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return Job{}, fmt.Errorf("session %s is not acp-backed", ref)
	}
	if loader, ok := m.store.(acpStateLoader); ok {
		if state, err := loader.LoadACPState(session.ID); err == nil {
			return jobFromInactiveState(session, state), nil
		}
	}
	return jobFromSession(session, session.RuntimeRef.Agent, session.RuntimeRef.SessionID, "", StateNotRunning), nil
}

func (m *Manager) configuredAgent(name string) (AgentConfig, bool, error) {
	return m.agents.AgentConfig(CanonicalAgentName(name))
}

func (m *Manager) Wait(ctx context.Context, req WaitRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, err
	}
	done := job.turnDone()
	job.mu.RLock()
	state := job.State
	job.mu.RUnlock()
	if state != StateRunning && state != StateStarting {
		return job.Snapshot(), nil
	}
	if req.Timeout <= 0 {
		req.Timeout = 30 * time.Second
	}
	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Job{}, ctx.Err()
	case <-timer.C:
		return job.Snapshot(), nil
	case <-done:
		return job.Snapshot(), nil
	}
}

// Cancel asks the agent to stop the current turn (session/cancel) and lets
// the turn end through the protocol so state and events stay consistent. If
// the agent doesn't stop in time, the process is torn down and removed so the
// next send resumes a fresh one. Cancel always succeeds for stored sessions:
// with no live job it clears the stuck stored state instead.
func (m *Manager) Cancel(ctx context.Context, ref string) (Job, error) {
	job, err := m.job(ref)
	if err != nil {
		return m.cancelStored(ref)
	}
	running, done := job.requestCancel()
	m.log.Info("acp cancel requested", "session", job.ID, "agent", job.ACPAgent, "running", running)
	if peer := m.peer(job.ID); peer != nil {
		if err := peer.Notify(ctx, acpschema.AgentMethodSessionCancel, acpschema.CancelNotification{
			SessionID: acpschema.SessionID(job.ACPSession),
		}); err != nil {
			m.log.Warn("acp cancel notify failed", "session", job.ID, "error", err)
		}
	} else if cancel := m.cancelFunc(job.ID); cancel != nil {
		cancel()
	}
	if !running || done == nil {
		return job.Snapshot(), nil
	}
	select {
	case <-done:
		m.log.Info("acp turn cancelled", "session", job.ID)
	case <-time.After(5 * time.Second):
		m.log.Warn("acp agent ignored cancel, tearing down process", "session", job.ID)
		m.teardown(job.ID)
		job.mu.RLock()
		stillRunning := job.State == StateRunning || job.State == StateStarting
		job.mu.RUnlock()
		if stillRunning {
			job.setState(StateCancelled, "cancelled", "")
			m.publishACPStatus(job.Snapshot())
		}
	case <-ctx.Done():
	}
	return job.Snapshot(), nil
}

// cancelStored unsticks a session that has no live job (server restarted
// mid-turn): the stored state flips to cancelled, which also resets the
// thread status, and a status event tells open pages to refresh.
func (m *Manager) cancelStored(ref string) (Job, error) {
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return Job{}, fmt.Errorf("session not found: %s", ref)
	}
	m.log.Info("cancel for inactive session, clearing stored state", "session", session.ID)
	var state storage.ACPState
	if loader, ok := m.store.(acpStateLoader); ok {
		state, _ = loader.LoadACPState(session.ID)
	}
	state.ID = session.ID
	state.Slug = firstNonEmpty(state.Slug, session.Slug)
	state.Title = firstNonEmpty(state.Title, session.Title)
	state.ParentID = firstNonEmpty(state.ParentID, session.ParentID)
	state.ModelProvider = firstNonEmpty(session.ModelProvider, state.ModelProvider)
	state.Model = firstNonEmpty(session.Model, state.Model)
	state.ReasoningEffort = firstNonEmpty(session.ReasoningEffort, state.ReasoningEffort)
	if session.RuntimeRef != nil {
		state.ACPAgent = firstNonEmpty(state.ACPAgent, session.RuntimeRef.Agent)
		state.ACPSession = firstNonEmpty(state.ACPSession, session.RuntimeRef.SessionID)
	}
	state.State = StateCancelled
	state.StopReason = "cancelled"
	state.ActiveOperation = ""
	state.Permissions = nil
	now := time.Now().UTC()
	state.UpdatedAt = now
	state.LastEventAt = now
	if saver, ok := m.store.(acpStateSaver); ok {
		if err := saver.SaveACPState(session.ID, state); err != nil {
			m.log.Warn("clearing stored acp state failed", "session", session.ID, "error", err)
		}
	}
	cancelled := Job{
		ID:              session.ID,
		Slug:            session.Slug,
		Title:           session.Title,
		ParentID:        session.ParentID,
		ACPAgent:        state.ACPAgent,
		ACPSession:      state.ACPSession,
		ModelProvider:   state.ModelProvider,
		Model:           state.Model,
		ReasoningEffort: state.ReasoningEffort,
		State:           StateCancelled,
		StopReason:      "cancelled",
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       now,
		LastEventAt:     now,
		LastToolAt:      state.LastToolAt,
	}
	m.publishOrderedACPEvents(cancelled, sessionevents.Event{
		SessionID: session.ID,
		Type:      "acp",
		ACP:       EventFromJob(cancelled),
	})
	return cancelled, nil
}

func (m *Manager) teardown(id string) {
	if job := m.jobByID(id); job != nil {
		m.withACPTranscriptBarrier(job.Snapshot(), nil)
	}
	m.transcriptBuffers.delete(id)
	m.mu.Lock()
	job := m.jobsByID[id]
	conn := m.connsByID[id]
	peer := m.peersByID[id]
	cancel := m.cancelByID[id]
	delete(m.jobsByID, id)
	delete(m.connsByID, id)
	delete(m.peersByID, id)
	delete(m.cancelByID, id)
	delete(m.serveErrByID, id)
	if job != nil {
		delete(m.jobsBySlug, job.Slug)
		delete(m.jobsByACP, job.ACPSession)
	}
	m.mu.Unlock()
	if peer != nil {
		_ = peer.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) List() []Job {
	m.mu.RLock()
	jobs := make([]*jobState, 0, len(m.jobsByID))
	for _, job := range m.jobsByID {
		jobs = append(jobs, job)
	}
	m.mu.RUnlock()
	out := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, job.Snapshot())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// Agents lists the names of configured ACP agents, sorted, so the new-thread
// UI can offer them as session runtimes.
func (m *Manager) Agents() []string {
	names, err := m.agents.EnabledAgentNames()
	if err != nil {
		m.log.Warn("loading configured acp agents failed", "error", err)
		return []string{}
	}
	return SelectableAgentNames(names)
}

func (m *Manager) Close() {
	m.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.cancelByID))
	peers := make([]*jsonrpc.Peer, 0, len(m.peersByID))
	conns := make([]jsonrpc.MessageConn, 0, len(m.connsByID))
	jobs := make([]*jobState, 0, len(m.jobsByID))
	for _, cancel := range m.cancelByID {
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	for _, peer := range m.peersByID {
		peers = append(peers, peer)
	}
	for _, conn := range m.connsByID {
		conns = append(conns, conn)
	}
	for _, job := range m.jobsByID {
		jobs = append(jobs, job)
	}
	m.connsByID = map[string]jsonrpc.MessageConn{}
	m.peersByID = map[string]*jsonrpc.Peer{}
	m.cancelByID = map[string]context.CancelFunc{}
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	for _, peer := range peers {
		_ = peer.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	for _, job := range jobs {
		job.setState(StateCancelled, "server_shutdown", "")
		m.withACPTranscriptBarrier(job.Snapshot(), nil)
		m.transcriptBuffers.delete(job.ID)
	}
}

func (m *Manager) addJob(job *jobState, conn jsonrpc.MessageConn, peer *jsonrpc.Peer, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsByID[job.ID] = job
	m.jobsBySlug[job.Slug] = job
	m.jobsByACP[job.ACPSession] = job
	if conn != nil {
		m.connsByID[job.ID] = conn
	}
	if peer != nil {
		m.peersByID[job.ID] = peer
	}
	if cancel != nil {
		m.cancelByID[job.ID] = cancel
	}
}

func (m *Manager) job(ref string) (*jobState, error) {
	ref = strings.TrimSpace(ref)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if job := m.jobsByID[ref]; job != nil {
		return job, nil
	}
	if job := m.jobsBySlug[ref]; job != nil {
		return job, nil
	}
	if job := m.jobsByACP[ref]; job != nil {
		return job, nil
	}
	return nil, fmt.Errorf("active acp session not found: %s", ref)
}

func (m *Manager) jobByACP(acpSessionID string) *jobState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByACP[acpSessionID]
}

func (m *Manager) jobByID(id string) *jobState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByID[id]
}

func (m *Manager) peer(id string) *jsonrpc.Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.peersByID[id]
}

func (m *Manager) setServeErr(peer *jsonrpc.Peer, err error) {
	var id string
	var job *jobState
	m.mu.Lock()
	for candidateID, candidate := range m.peersByID {
		if candidate == peer {
			id = candidateID
			m.serveErrByID[id] = err
			job = m.jobsByID[id]
			break
		}
	}
	m.mu.Unlock()
	if id == "" {
		return
	}
	m.log.Error("acp agent connection failed", "session", id, "error", err)
	if job == nil {
		return
	}
	job.mu.RLock()
	running := job.State == StateRunning || job.State == StateStarting
	job.mu.RUnlock()
	if running {
		return
	}
	job.setState(StateFailed, "", err.Error())
	m.publishACPStatus(job.Snapshot())
}

func (m *Manager) serveErr(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serveErrByID[id]
}
