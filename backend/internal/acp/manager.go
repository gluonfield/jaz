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
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	StateStarting  = "starting"
	StateRunning   = "running"
	StateIdle      = "idle"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
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
	jobsByID     map[string]*Job
	jobsBySlug   map[string]*Job
	jobsByACP    map[string]*Job
	connsByID    map[string]jsonrpc.MessageConn
	peersByID    map[string]*jsonrpc.Peer
	cancelByID   map[string]context.CancelFunc
	serveErrByID map[string]error

	permissionSeq     uint64
	pendingPermission map[string]*pendingPermission
	permissionMu      sync.Mutex

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
	// Model overrides the agent's configured model for this session (empty
	// keeps the agent default).
	Model string
	// ReasoningEffort overrides the agent's configured reasoning effort for this
	// session (empty keeps the agent default).
	ReasoningEffort string
	// SourceType/SourceID tag sessions created by backend automation such as loops.
	SourceType string
	SourceID   string
}

type SendRequest struct {
	Session       string
	Message       string
	Attachments   []storage.Attachment
	Completion    CompletionMode
	Interactive   bool
	PlanRequested bool
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
		jobsByID:          make(map[string]*Job),
		jobsBySlug:        make(map[string]*Job),
		jobsByACP:         make(map[string]*Job),
		connsByID:         make(map[string]jsonrpc.MessageConn),
		peersByID:         make(map[string]*jsonrpc.Peer),
		cancelByID:        make(map[string]context.CancelFunc),
		serveErrByID:      make(map[string]error),
		pendingPermission: make(map[string]*pendingPermission),
	}
}

type agentConn struct {
	conn    jsonrpc.MessageConn
	peer    *jsonrpc.Peer
	cancel  context.CancelFunc
	initRaw json.RawMessage
}

func (c *agentConn) close() {
	_ = c.peer.Close()
	c.cancel()
}

func (m *Manager) connect(ctx context.Context, name string, cfg AgentConfig, cwd string) (*agentConn, error) {
	env := m.processEnv(name, cfg)
	runCtx, cancel := context.WithCancel(context.Background())
	conn, err := m.openConn(runCtx, name, cfg, env, cwd)
	if err != nil {
		cancel()
		return nil, err
	}
	peer := jsonrpc.NewPeer(conn, jsonrpc.HandlerFunc(m.handleJSONRPC))
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
		},
	})
	if err != nil {
		_ = peer.Close()
		cancel()
		return nil, fmt.Errorf("initialize acp agent: %w", err)
	}
	methodID, missingAuth := autoAuthMethod(name, initRaw, env)
	if methodID != "" {
		if _, err := peer.Call(ctx, acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID}); err != nil {
			_ = peer.Close()
			cancel()
			return nil, fmt.Errorf("authenticate acp agent: %w", err)
		}
	} else if len(missingAuth) > 0 {
		_ = peer.Close()
		cancel()
		return nil, fmt.Errorf("authenticate acp agent %q: missing %s", name, strings.Join(missingAuth, " or "))
	}
	return &agentConn{conn: conn, peer: peer, cancel: cancel, initRaw: initRaw}, nil
}

// sessionPromptMeta builds the _meta payload carrying the Jaz system prompt
// in the form the named agent understands, or nil when no prompt is
// configured.
func (m *Manager) sessionPromptMeta(agent string) (map[string]any, error) {
	if m.cfg.SystemPrompt == nil {
		return nil, nil
	}
	prompt, err := m.cfg.SystemPrompt.ACPPrompt()
	if err != nil {
		return nil, fmt.Errorf("build acp system prompt: %w", err)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, nil
	}
	return systemPromptMeta(agent, prompt), nil
}

func (m *Manager) newACPSession(ctx context.Context, ac *agentConn, agent, cwd string) (acpSessionInfo, error) {
	meta, err := m.sessionPromptMeta(agent)
	if err != nil {
		return acpSessionInfo{}, err
	}
	newSession := struct {
		Meta       map[string]any    `json:"_meta,omitempty"`
		Cwd        string            `json:"cwd"`
		MCPServers []json.RawMessage `json:"mcpServers"`
	}{
		Meta:       meta,
		Cwd:        cwd,
		MCPServers: m.mcpServersForAgent(ac.initRaw),
	}
	sessionRaw, err := ac.peer.Call(ctx, acpschema.AgentMethodSessionNew, newSession)
	if err != nil {
		return acpSessionInfo{}, fmt.Errorf("create acp session: %w", err)
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

func (m *Manager) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	req.ACPAgent = CanonicalAgentName(req.ACPAgent)
	if req.ACPAgent == "" {
		req.ACPAgent = AgentCodex
	}
	cfg, ok, err := m.configuredAgent(req.ACPAgent)
	if err != nil {
		return SpawnResult{}, err
	}
	if !ok {
		return SpawnResult{}, fmt.Errorf("acp agent %q is not configured", req.ACPAgent)
	}
	effort := configuredReasoningEffort(cfg.ReasoningEffort)
	if req.ReasoningEffort != "" {
		effort = req.ReasoningEffort
	}
	// Apply the effective model and effort (per-request overrides win) to the
	// agent config so configuredModeState pushes them to the agent, not just
	// the session record.
	if model := strings.TrimSpace(req.Model); model != "" {
		cfg.Model = model
	}
	cfg.ReasoningEffort = effort
	// The session row is created first: its unique slug names the default
	// directory and the worktree branch.
	session, err := m.store.CreateSession(storage.CreateSession{
		Slug:            req.Slug,
		Title:           req.Title,
		ParentID:        req.ParentID,
		Runtime:         storage.RuntimeACP,
		ModelProvider:   req.ACPAgent,
		Model:           strings.TrimSpace(cfg.Model),
		ReasoningEffort: effort,
		SourceType:      req.SourceType,
		SourceID:        req.SourceID,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: req.ACPAgent,
		},
	})
	if err != nil {
		return SpawnResult{}, err
	}
	fail := func(err error) (SpawnResult, error) {
		session.Status = storage.StatusError
		session.Error = err.Error()
		_ = m.store.SaveSession(session)
		return SpawnResult{}, err
	}
	absCwd, projectPath, err := m.prepareSessionDir(req, cfg, session.Slug)
	if err != nil {
		return fail(err)
	}
	ac, err := m.connect(ctx, req.ACPAgent, cfg, absCwd)
	if err != nil {
		return fail(err)
	}
	acpSession, err := m.newACPSession(ctx, ac, req.ACPAgent, absCwd)
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
	session.RuntimeRef.ProjectPath = projectPath
	if err := m.store.SaveSession(session); err != nil {
		ac.close()
		return fail(err)
	}
	job := &Job{
		ID:         session.ID,
		Slug:       session.Slug,
		Title:      session.Title,
		ParentID:   session.ParentID,
		ACPAgent:   req.ACPAgent,
		ACPSession: string(acpSession.response.SessionID),
		Cwd:        absCwd,
		State:      StateIdle,
		Modes:      modes,
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
		toolByID:   make(map[string]ToolCallSnapshot),
	}
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
	if acpModes == nil && CanonicalAgentName(agentName) == AgentGrok {
		acpModes = grokFallbackModes()
	}
	modes := modeStateFromACP(acpModes)
	if acpModes == nil {
		return modes, nil
	}
	if modes.ExecutionModeID == "" {
		modes.ExecutionModeID = modes.CurrentModeID
	}
	if preferred := preferredExecutionMode(acpModes.AvailableModes); preferred != "" {
		modes.ExecutionModeID = preferred
		if modes.CurrentModeID != preferred {
			if err := m.setSessionMode(ctx, peer, session.SessionID, preferred); err != nil {
				return modes, err
			}
			modes.CurrentModeID = preferred
		}
	}
	return modes, nil
}

// Restarts the agent for a stored session (server restart): session/load when
// supported, otherwise a fresh agent session in the same workspace.
func (m *Manager) resume(ctx context.Context, ref string) (*Job, error) {
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
	agentName := CanonicalAgentName(session.RuntimeRef.Agent)
	sessionChanged := false
	if agentName != session.RuntimeRef.Agent {
		if session.ModelProvider == session.RuntimeRef.Agent {
			session.ModelProvider = agentName
		}
		session.RuntimeRef.Agent = agentName
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
	cfg.ReasoningEffort = strings.TrimSpace(session.ReasoningEffort)
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
	ac, err := m.connect(ctx, agentName, cfg, cwd)
	if err != nil {
		return nil, err
	}
	acpSessionID, modes, err := m.restoreACPSession(ctx, ac, agentName, session, cfg, cwd)
	if err != nil {
		ac.close()
		return nil, err
	}
	if acpSessionID != session.RuntimeRef.SessionID {
		session.RuntimeRef.SessionID = acpSessionID
		sessionChanged = true
	}
	if session.ModelProvider == "" {
		session.ModelProvider = session.RuntimeRef.Agent
		sessionChanged = true
	}
	if sessionChanged {
		_ = m.store.SaveSession(session)
	}
	job := &Job{
		ID:            session.ID,
		Slug:          session.Slug,
		Title:         session.Title,
		ParentID:      session.ParentID,
		ACPAgent:      agentName,
		ACPSession:    acpSessionID,
		Cwd:           cwd,
		State:         StateIdle,
		Modes:         modes,
		ParentVisible: state.ParentVisible,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     time.Now().UTC(),
		toolByID:      make(map[string]ToolCallSnapshot),
	}
	m.addJob(job, ac.conn, ac.peer, ac.cancel)
	m.saveACPState(job.Snapshot())
	m.log.Info("resumed agent session", "agent", job.ACPAgent, "session", job.ID,
		"acp_session", acpSessionID, "loaded", acpSessionID == session.RuntimeRef.SessionID)
	return job, nil
}

// The job is registered only after session/load returns, so the agent's
// history replay notifications are dropped, not re-recorded as events.
func (m *Manager) restoreACPSession(ctx context.Context, ac *agentConn, agentName string, session storage.Session, cfg AgentConfig, cwd string) (string, ModeState, error) {
	agentName = CanonicalAgentName(agentName)
	var caps struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	_ = json.Unmarshal(ac.initRaw, &caps)
	storedID := session.RuntimeRef.SessionID
	if caps.AgentCapabilities.LoadSession && storedID != "" {
		meta, err := m.sessionPromptMeta(agentName)
		if err != nil {
			return "", ModeState{}, err
		}
		raw, err := ac.peer.Call(ctx, acpschema.AgentMethodSessionLoad, struct {
			Meta       map[string]any      `json:"_meta,omitempty"`
			Cwd        string              `json:"cwd"`
			MCPServers []json.RawMessage   `json:"mcpServers"`
			SessionID  acpschema.SessionID `json:"sessionId"`
		}{
			Meta:       meta,
			Cwd:        cwd,
			MCPServers: m.mcpServersForAgent(ac.initRaw),
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
	acpSession, err := m.newACPSession(ctx, ac, agentName, cwd)
	if err != nil {
		return "", ModeState{}, err
	}
	modes, err := m.configuredModeState(ctx, ac.peer, agentName, acpSession, cfg)
	return string(acpSession.response.SessionID), modes, err
}

func (m *Manager) Send(ctx context.Context, req SendRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		if job, err = m.resume(ctx, req.Session); err != nil {
			return Job{}, err
		}
	}
	if strings.TrimSpace(req.Message) == "" {
		return Job{}, fmt.Errorf("message is required")
	}
	job.mu.RLock()
	state := job.State
	job.mu.RUnlock()
	if state == StateRunning || state == StateStarting {
		return Job{}, fmt.Errorf("session %s is already running", job.Slug)
	}
	if err := m.prepareModeForTurn(ctx, job, req.PlanRequested); err != nil {
		return Job{}, err
	}
	_ = storage.AppendUserMessage(m.store, job.ID, req.Message, req.Attachments)
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested)
	job.startTurn(req.Completion, req.Interactive, req.PlanRequested, req.ParentVisible)
	m.touchJobAttention(job)
	m.publishACP(job.Snapshot())
	go m.runPrompt(context.Background(), job, req.Message, req.Attachments)
	return job.Snapshot(), nil
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
	return Job{
		ID:         session.ID,
		Slug:       session.Slug,
		Title:      session.Title,
		ParentID:   session.ParentID,
		ACPAgent:   CanonicalAgentName(session.RuntimeRef.Agent),
		ACPSession: session.RuntimeRef.SessionID,
		State:      "not_running",
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  session.UpdatedAt,
	}, nil
}

func (m *Manager) configuredAgent(name string) (AgentConfig, bool, error) {
	return m.agents.AgentConfig(CanonicalAgentName(name))
}

func (m *Manager) Wait(ctx context.Context, req WaitRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, err
	}
	job.mu.RLock()
	done := job.done
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
	job.mu.Lock()
	job.cancelRequested = true
	running := job.State == StateRunning || job.State == StateStarting
	done := job.done
	job.mu.Unlock()
	m.log.Info("acp cancel requested", "session", job.ID, "agent", job.ACPAgent, "running", running)
	if peer := m.peer(job.ID); peer != nil {
		if err := peer.Notify(ctx, acpschema.AgentMethodSessionCancel, acpschema.CancelNotification{
			SessionID: acpschema.SessionID(job.ACPSession),
		}); err != nil {
			m.log.Warn("acp cancel notify failed", "session", job.ID, "error", err)
		}
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
	if session.RuntimeRef != nil {
		state.ACPAgent = firstNonEmpty(state.ACPAgent, session.RuntimeRef.Agent)
		state.ACPSession = firstNonEmpty(state.ACPSession, session.RuntimeRef.SessionID)
	}
	state.State = StateCancelled
	state.StopReason = "cancelled"
	if saver, ok := m.store.(acpStateSaver); ok {
		if err := saver.SaveACPState(session.ID, state); err != nil {
			m.log.Warn("clearing stored acp state failed", "session", session.ID, "error", err)
		}
	}
	m.recordAndPublish(sessionevents.Event{
		SessionID: session.ID,
		Type:      "acp",
		ACP: &sessionevents.ACPEvent{
			ID:         session.ID,
			Slug:       state.Slug,
			Title:      state.Title,
			ParentID:   state.ParentID,
			Agent:      state.ACPAgent,
			SessionID:  state.ACPSession,
			State:      StateCancelled,
			StopReason: "cancelled",
		},
	})
	return Job{
		ID:         session.ID,
		Slug:       session.Slug,
		Title:      session.Title,
		ParentID:   session.ParentID,
		ACPAgent:   state.ACPAgent,
		ACPSession: state.ACPSession,
		State:      StateCancelled,
		StopReason: "cancelled",
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
	}, nil
}

func (m *Manager) teardown(id string) {
	m.mu.Lock()
	job := m.jobsByID[id]
	conn := m.connsByID[id]
	peer := m.peersByID[id]
	cancel := m.cancelByID[id]
	delete(m.jobsByID, id)
	delete(m.connsByID, id)
	delete(m.peersByID, id)
	delete(m.cancelByID, id)
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
	jobs := make([]*Job, 0, len(m.jobsByID))
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
	sort.Strings(names)
	return names
}

func (m *Manager) Close() {
	m.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.cancelByID))
	peers := make([]*jsonrpc.Peer, 0, len(m.peersByID))
	conns := make([]jsonrpc.MessageConn, 0, len(m.connsByID))
	jobs := make([]*Job, 0, len(m.jobsByID))
	for _, cancel := range m.cancelByID {
		cancels = append(cancels, cancel)
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
	}
}

func (m *Manager) addJob(job *Job, conn jsonrpc.MessageConn, peer *jsonrpc.Peer, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsByID[job.ID] = job
	m.jobsBySlug[job.Slug] = job
	m.jobsByACP[job.ACPSession] = job
	m.connsByID[job.ID] = conn
	m.peersByID[job.ID] = peer
	m.cancelByID[job.ID] = cancel
}

func (m *Manager) job(ref string) (*Job, error) {
	ref = strings.TrimSpace(ref)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if job := m.jobsByID[ref]; job != nil {
		return job, nil
	}
	if job := m.jobsBySlug[ref]; job != nil {
		return job, nil
	}
	return nil, fmt.Errorf("active acp session not found: %s", ref)
}

func (m *Manager) jobByACP(acpSessionID string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByACP[acpSessionID]
}

func (m *Manager) jobByID(id string) *Job {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, candidate := range m.peersByID {
		if candidate == peer {
			m.serveErrByID[id] = err
			m.log.Error("acp agent connection failed", "session", id, "error", err)
			if job := m.jobsByID[id]; job != nil {
				job.setState(StateFailed, "", err.Error())
			}
			return
		}
	}
}
