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
	"github.com/wins/jaz/backend/internal/provider"
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
	storage.MessageAppender
	storage.SessionEventAppender
	storage.ActivityUpserter
}

type Manager struct {
	cfg          Config
	store        Store
	log          *log.Logger
	Done         func(context.Context, Job)
	TurnFinished func(context.Context, Job)
	Events       *sessionevents.Bus

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
	// SourceType/SourceID tag sessions created by backend automation such as loops.
	SourceType string
	SourceID   string
}

type SendRequest struct {
	Session       string
	Message       string
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
}

func NewManager(store Store, cfg Config, logger *log.Logger) *Manager {
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{
		cfg:               cfg,
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

func (m *Manager) newACPSession(ctx context.Context, ac *agentConn, cwd string) (acpSessionInfo, error) {
	newSession := struct {
		Meta       map[string]any    `json:"_meta,omitempty"`
		Cwd        string            `json:"cwd"`
		MCPServers []json.RawMessage `json:"mcpServers"`
	}{
		Cwd:        cwd,
		MCPServers: m.mcpServersForAgent(ac.initRaw),
	}
	if m.cfg.SystemPrompt != nil {
		if prompt := strings.TrimSpace(m.cfg.SystemPrompt.SkillsPrompt()); prompt != "" {
			newSession.Meta = map[string]any{"systemPrompt": prompt}
		}
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
	req.ACPAgent = strings.TrimSpace(req.ACPAgent)
	if req.ACPAgent == "" {
		req.ACPAgent = "codex"
	}
	cfg, ok := m.cfg.Agent(req.ACPAgent)
	if !ok {
		return SpawnResult{}, fmt.Errorf("acp agent %q is not configured", req.ACPAgent)
	}
	// The session row is created first: its unique slug names the default
	// directory and the worktree branch.
	session, err := m.store.CreateSession(storage.CreateSession{
		Slug:            req.Slug,
		Title:           req.Title,
		ParentID:        req.ParentID,
		Runtime:         storage.RuntimeACP,
		ModelProvider:   req.ACPAgent,
		Model:           strings.TrimSpace(cfg.Model),
		ReasoningEffort: configuredReasoningEffort(cfg.ReasoningEffort),
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
	absCwd, err := m.prepareSessionDir(req, cfg, session.Slug)
	if err != nil {
		return fail(err)
	}
	ac, err := m.connect(ctx, req.ACPAgent, cfg, absCwd)
	if err != nil {
		return fail(err)
	}
	acpSession, err := m.newACPSession(ctx, ac, absCwd)
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
	}, nil
}

func (m *Manager) initializeModeState(ctx context.Context, peer *jsonrpc.Peer, session acpschema.NewSessionResponse) (ModeState, error) {
	modes := modeStateFromACP(session.Modes)
	if session.Modes == nil {
		return modes, nil
	}
	if modes.ExecutionModeID == "" {
		modes.ExecutionModeID = modes.CurrentModeID
	}
	if preferred := preferredExecutionMode(session.Modes.AvailableModes); preferred != "" {
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
	cfg, ok := m.cfg.Agent(session.RuntimeRef.Agent)
	if !ok {
		return nil, fmt.Errorf("acp agent %q is not configured", session.RuntimeRef.Agent)
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
	ac, err := m.connect(ctx, session.RuntimeRef.Agent, cfg, cwd)
	if err != nil {
		return nil, err
	}
	acpSessionID, modes, err := m.restoreACPSession(ctx, ac, session, cfg, cwd)
	if err != nil {
		ac.close()
		return nil, err
	}
	sessionChanged := false
	if acpSessionID != session.RuntimeRef.SessionID {
		session.RuntimeRef.SessionID = acpSessionID
		sessionChanged = true
	}
	if session.ModelProvider == "" {
		session.ModelProvider = session.RuntimeRef.Agent
		sessionChanged = true
	}
	if model := strings.TrimSpace(cfg.Model); model != "" && session.Model != model {
		session.Model = model
		sessionChanged = true
	}
	if effort := configuredReasoningEffort(cfg.ReasoningEffort); effort != "" && session.ReasoningEffort != effort {
		session.ReasoningEffort = effort
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
		ACPAgent:      session.RuntimeRef.Agent,
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
func (m *Manager) restoreACPSession(ctx context.Context, ac *agentConn, session storage.Session, cfg AgentConfig, cwd string) (string, ModeState, error) {
	var caps struct {
		AgentCapabilities acpschema.AgentCapabilities `json:"agentCapabilities"`
	}
	_ = json.Unmarshal(ac.initRaw, &caps)
	storedID := session.RuntimeRef.SessionID
	if caps.AgentCapabilities.LoadSession && storedID != "" {
		raw, err := ac.peer.Call(ctx, acpschema.AgentMethodSessionLoad, struct {
			Meta       map[string]any      `json:"_meta,omitempty"`
			Cwd        string              `json:"cwd"`
			MCPServers []json.RawMessage   `json:"mcpServers"`
			SessionID  acpschema.SessionID `json:"sessionId"`
		}{
			Cwd:        cwd,
			MCPServers: m.mcpServersForAgent(ac.initRaw),
			SessionID:  acpschema.SessionID(storedID),
		})
		if err == nil {
			var resp acpschema.LoadSessionResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return "", ModeState{}, err
			}
			modes, err := m.configuredModeState(ctx, ac.peer, session.RuntimeRef.Agent, newACPSessionInfo(raw, acpschema.NewSessionResponse{
				SessionID: acpschema.SessionID(storedID),
				Modes:     resp.Modes,
			}), cfg)
			return storedID, modes, err
		}
		// The agent lost this session — fall through to a fresh one.
	}
	acpSession, err := m.newACPSession(ctx, ac, cwd)
	if err != nil {
		return "", ModeState{}, err
	}
	modes, err := m.configuredModeState(ctx, ac.peer, session.RuntimeRef.Agent, acpSession, cfg)
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
	_ = m.store.AppendMessages(job.ID, provider.UserMessage(req.Message))
	m.log.Info("acp turn started", "session", job.ID, "agent", job.ACPAgent, "plan", req.PlanRequested)
	job.startTurn(req.Completion, req.Interactive, req.PlanRequested, req.ParentVisible)
	m.publishACP(job.Snapshot())
	go m.runPrompt(context.Background(), job, req.Message)
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
		ACPAgent:   session.RuntimeRef.Agent,
		ACPSession: session.RuntimeRef.SessionID,
		State:      "not_running",
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  session.UpdatedAt,
	}, nil
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

func (m *Manager) runPrompt(ctx context.Context, job *Job, message string) {
	job.turnMu.Lock()
	defer job.turnMu.Unlock()

	job.mu.RLock()
	done := job.done
	job.mu.RUnlock()
	if done == nil {
		done = job.startTurn(CompletionInline, false, false, false)
	}

	peer := m.peer(job.ID)
	if peer == nil {
		m.failTurn(job, fmt.Errorf("acp peer is not active"))
		m.finishTurn(done, job)
		return
	}
	raw, err := peer.Call(ctx, acpschema.AgentMethodSessionPrompt, map[string]any{
		"sessionId": job.ACPSession,
		"prompt": []any{
			map[string]any{"type": "text", "text": message},
		},
	})
	if err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	if usage := usageFromRaw(raw); !usageEmpty(usage) {
		m.recordUsage(job, usage)
	}
	stopReason := resp.StopReason
	state := StateIdle
	if stopReason == "cancelled" {
		state = StateCancelled
	}
	job.setState(state, stopReason, "")
	m.log.Info("acp turn finished", "session", job.ID, "state", state, "stop_reason", stopReason)
	m.publishACPStatus(job.Snapshot())
	time.Sleep(50 * time.Millisecond)
	m.persistUsage(job)
	m.appendAssistantMessage(job)
	m.finishTurn(done, job)
}

// A turn that died after a cancel request ends as cancelled, not failed; both
// outcomes are published so the UI and stored status reflect them.
func (m *Manager) failTurn(job *Job, err error) {
	job.mu.RLock()
	cancelled := job.cancelRequested
	job.mu.RUnlock()
	if cancelled {
		job.setState(StateCancelled, "cancelled", "")
		m.log.Info("acp turn cancelled", "session", job.ID)
	} else {
		job.setState(StateFailed, "", err.Error())
		m.log.Error("acp turn failed", "session", job.ID, "error", err)
	}
	m.publishACPStatus(job.Snapshot())
}

func (m *Manager) finishTurn(done chan struct{}, job *Job) {
	job.mu.Lock()
	completion := job.completion
	planRequested := job.planRequested
	parentVisible := job.ParentVisible
	job.completion = CompletionInline
	job.interactive = false
	job.planRequested = false
	job.mu.Unlock()
	m.cancelPendingPermissions(job.ID)
	m.resolveDanglingToolCalls(job)
	snapshot := job.Snapshot()
	if m.TurnFinished != nil {
		m.TurnFinished(context.Background(), snapshot)
	}
	close(done)
	if completion.propagates() && parentVisible && !planRequested && m.Done != nil {
		go m.Done(context.Background(), snapshot)
	}
}

// A cancelled or failed turn leaves the agent's in-flight tool calls without
// terminal updates; resolve them so they don't render as running forever.
func (m *Manager) resolveDanglingToolCalls(job *Job) {
	job.mu.Lock()
	state := job.State
	if state != StateCancelled && state != StateFailed {
		job.mu.Unlock()
		return
	}
	status := "cancelled"
	if state == StateFailed {
		status = "failed"
	}
	var updated []ToolCallSnapshot
	for id, call := range job.toolByID {
		if terminalToolStatus(call.Status) {
			continue
		}
		call.Status = status
		job.toolByID[id] = call
		updated = append(updated, call)
	}
	if len(updated) == 0 {
		job.mu.Unlock()
		return
	}
	job.ToolCalls = sortedToolCalls(job.toolByID)
	job.UpdatedAt = time.Now().UTC()
	sessionID := job.ID
	job.mu.Unlock()
	m.log.Info("resolved dangling tool calls", "session", sessionID, "count", len(updated), "status", status)
	for _, call := range updated {
		_ = m.store.UpsertActivity(sessionID, storage.ActivityEntry{
			ID:     call.ID,
			Kind:   "tool",
			Text:   firstNonEmpty(call.Title, call.ID),
			Status: call.Status,
			At:     time.Now().UTC(),
		})
		m.publishACPTool(job.Snapshot(), call)
	}
}

func terminalToolStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "failed", "cancelled", "canceled":
		return true
	}
	return false
}

func (m *Manager) appendAssistantMessage(job *Job) {
	job.mu.Lock()
	if job.planRequested {
		job.mu.Unlock()
		return
	}
	if job.savedAssistantLen >= len(job.Assistant) {
		job.mu.Unlock()
		return
	}
	content := job.Assistant[job.savedAssistantLen:]
	job.savedAssistantLen = len(job.Assistant)
	sessionID := job.ID
	job.mu.Unlock()
	if strings.TrimSpace(content) == "" {
		return
	}
	_ = m.store.AppendMessages(sessionID, provider.AssistantMessage(content, nil))
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
