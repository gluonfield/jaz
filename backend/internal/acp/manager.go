package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/provider"
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
	AppendMessages(string, ...provider.Message) error
	UpsertActivity(string, storage.ActivityEntry) error
}

type Manager struct {
	cfg   Config
	store Store
	Done  func(context.Context, Job)

	mu           sync.RWMutex
	jobsByID     map[string]*Job
	jobsBySlug   map[string]*Job
	jobsByACP    map[string]*Job
	connsByID    map[string]jsonrpc.MessageConn
	peersByID    map[string]*jsonrpc.Peer
	cancelByID   map[string]context.CancelFunc
	serveErrByID map[string]error
}

type SpawnRequest struct {
	ParentID string
	ACPAgent string
	Slug     string
	Title    string
}

type SendRequest struct {
	Session    string
	Message    string
	Completion CompletionMode
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
	State     string `json:"state"`
}

func NewManager(store Store, cfg Config) *Manager {
	if cfg.Agents == nil {
		cfg.Agents = map[string]AgentConfig{}
	}
	cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt)
	return &Manager{
		cfg:          cfg,
		store:        store,
		jobsByID:     make(map[string]*Job),
		jobsBySlug:   make(map[string]*Job),
		jobsByACP:    make(map[string]*Job),
		connsByID:    make(map[string]jsonrpc.MessageConn),
		peersByID:    make(map[string]*jsonrpc.Peer),
		cancelByID:   make(map[string]context.CancelFunc),
		serveErrByID: make(map[string]error),
	}
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
	absCwd, err := m.resolveCwd(cfg.Cwd)
	if err != nil {
		return SpawnResult{}, err
	}
	env := m.processEnv(req.ACPAgent, cfg)

	runCtx, cancel := context.WithCancel(context.Background())
	conn, err := m.openConn(runCtx, req.ACPAgent, cfg, env, absCwd)
	if err != nil {
		cancel()
		return SpawnResult{}, err
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
		return SpawnResult{}, fmt.Errorf("initialize acp agent: %w", err)
	}
	methodID, missingAuth := autoAuthMethod(req.ACPAgent, initRaw, env)
	if methodID != "" {
		if _, err := peer.Call(ctx, acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID}); err != nil {
			_ = peer.Close()
			cancel()
			return SpawnResult{}, fmt.Errorf("authenticate acp agent: %w", err)
		}
	} else if len(missingAuth) > 0 {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, fmt.Errorf("authenticate acp agent %q: missing %s", req.ACPAgent, strings.Join(missingAuth, " or "))
	}
	newSession := acpschema.NewSessionRequest{
		Cwd:        absCwd,
		MCPServers: []acpschema.MCPServer{},
	}
	if m.cfg.SystemPrompt != "" {
		newSession.Meta = map[string]any{"systemPrompt": m.cfg.SystemPrompt}
	}
	sessionRaw, err := peer.Call(ctx, acpschema.AgentMethodSessionNew, newSession)
	if err != nil {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, fmt.Errorf("create acp session: %w", err)
	}
	var acpSession acpschema.NewSessionResponse
	if err := json.Unmarshal(sessionRaw, &acpSession); err != nil {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, err
	}
	if acpSession.SessionID == "" {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, fmt.Errorf("acp session/new returned empty session id")
	}
	if err := requireFullAccessMode(ctx, peer, acpSession); err != nil {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, err
	}

	session, err := m.store.CreateSession(storage.CreateSession{
		Slug:     req.Slug,
		Title:    req.Title,
		ParentID: req.ParentID,
		Runtime:  storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     req.ACPAgent,
			SessionID: string(acpSession.SessionID),
		},
	})
	if err != nil {
		_ = peer.Close()
		cancel()
		return SpawnResult{}, err
	}
	job := &Job{
		ID:         session.ID,
		Slug:       session.Slug,
		Title:      session.Title,
		ParentID:   session.ParentID,
		ACPAgent:   req.ACPAgent,
		ACPSession: string(acpSession.SessionID),
		Cwd:        absCwd,
		State:      StateIdle,
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
		toolByID:   make(map[string]ToolCallSnapshot),
	}
	m.addJob(job, conn, peer, cancel)

	return SpawnResult{
		Status:    "created",
		SessionID: job.ID,
		Slug:      job.Slug,
		ACPAgent:  job.ACPAgent,
		State:     StateIdle,
	}, nil
}

func requireFullAccessMode(ctx context.Context, peer *jsonrpc.Peer, session acpschema.NewSessionResponse) error {
	if session.Modes == nil {
		return nil
	}
	if slices.Contains(fullAccessModes, string(session.Modes.CurrentModeID)) {
		return nil
	}
	for _, id := range fullAccessModes {
		for _, mode := range session.Modes.AvailableModes {
			if string(mode.ID) != id {
				continue
			}
			_, err := peer.Call(ctx, acpschema.AgentMethodSessionSetMode, acpschema.SetSessionModeRequest{
				SessionID: session.SessionID,
				ModeID:    mode.ID,
			})
			if err != nil {
				return fmt.Errorf("set acp session %q mode: %w", id, err)
			}
			return nil
		}
	}
	return fmt.Errorf("acp session exposes no full-access mode (looked for %s); current mode is %q",
		strings.Join(fullAccessModes, ", "), session.Modes.CurrentModeID)
}

func (m *Manager) Send(ctx context.Context, req SendRequest) (Job, error) {
	job, err := m.job(req.Session)
	if err != nil {
		return Job{}, err
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
	_ = m.store.AppendMessages(job.ID, provider.UserMessage(req.Message))
	job.startTurn(req.Completion)
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

func (m *Manager) Cancel(ctx context.Context, ref string) (Job, error) {
	job, err := m.job(ref)
	if err != nil {
		return Job{}, err
	}
	peer := m.peer(job.ID)
	if peer != nil {
		_ = peer.Notify(ctx, acpschema.AgentMethodSessionCancel, acpschema.CancelNotification{
			SessionID: acpschema.SessionID(job.ACPSession),
		})
	}
	m.mu.RLock()
	cancel := m.cancelByID[job.ID]
	m.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	job.setState(StateCancelled, "cancelled", "")
	return job.Snapshot(), nil
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
		done = job.startTurn(CompletionInline)
	}

	peer := m.peer(job.ID)
	if peer == nil {
		job.setState(StateFailed, "", "acp peer is not active")
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
		job.setState(StateFailed, "", err.Error())
		m.finishTurn(done, job)
		return
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		job.setState(StateFailed, "", err.Error())
		m.finishTurn(done, job)
		return
	}
	stopReason := resp.StopReason
	state := StateIdle
	if stopReason == "cancelled" {
		state = StateCancelled
	}
	job.setState(state, stopReason, "")
	time.Sleep(50 * time.Millisecond)
	m.appendAssistantMessage(job)
	m.finishTurn(done, job)
}

func (m *Manager) finishTurn(done chan struct{}, job *Job) {
	job.mu.Lock()
	completion := job.completion
	job.completion = CompletionInline
	job.mu.Unlock()
	close(done)
	if completion.propagates() && m.Done != nil {
		go m.Done(context.Background(), job.Snapshot())
	}
}

func (m *Manager) appendAssistantMessage(job *Job) {
	job.mu.Lock()
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
			if job := m.jobsByID[id]; job != nil {
				job.setState(StateFailed, "", err.Error())
			}
			return
		}
	}
}
