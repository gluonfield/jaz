package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	acpschema "github.com/wins/acp-transport/acp"
	"github.com/wins/acp-transport/jsonrpc"
	"github.com/wins/acp-transport/stdio"
	"github.com/wins/acp-transport/streamhttp"
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
	LoadMessages(string) ([]provider.Message, error)
	SaveMessages(string, []provider.Message) error
}

type Manager struct {
	cfg   Config
	store Store

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
	Message  string
	Cwd      string
}

type SendRequest struct {
	Session string
	Message string
}

type WaitRequest struct {
	Session string
	Timeout time.Duration
}

type Job struct {
	ID         string             `json:"id"`
	Slug       string             `json:"slug"`
	Title      string             `json:"title,omitempty"`
	ParentID   string             `json:"parent_id,omitempty"`
	ACPAgent   string             `json:"acp_agent"`
	ACPSession string             `json:"acp_session"`
	Cwd        string             `json:"cwd,omitempty"`
	State      string             `json:"state"`
	StopReason string             `json:"stop_reason,omitempty"`
	Assistant  string             `json:"assistant,omitempty"`
	Thought    string             `json:"thought,omitempty"`
	Plan       []PlanEntry        `json:"plan,omitempty"`
	ToolCalls  []ToolCallSnapshot `json:"tool_calls,omitempty"`
	Error      string             `json:"error,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`

	mu                sync.RWMutex
	turnMu            sync.Mutex
	done              chan struct{}
	toolByID          map[string]ToolCallSnapshot
	savedAssistantLen int
}

type PlanEntry struct {
	Content  string `json:"content"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type ToolCallSnapshot struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
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
	if strings.TrimSpace(req.Message) == "" {
		return SpawnResult{}, fmt.Errorf("message is required")
	}
	cwd := firstNonEmpty(req.Cwd, cfg.Cwd)
	if cwd == "" {
		cwd = "."
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return SpawnResult{}, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	conn, err := m.openConn(runCtx, req.ACPAgent, cfg)
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
	if methodID := autoAuthMethod(initRaw); methodID != "" {
		if _, err := peer.Call(ctx, acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: methodID}); err != nil {
			_ = peer.Close()
			cancel()
			return SpawnResult{}, fmt.Errorf("authenticate acp agent: %w", err)
		}
	}
	sessionRaw, err := peer.Call(ctx, acpschema.AgentMethodSessionNew, acpschema.NewSessionRequest{
		Cwd:        absCwd,
		MCPServers: []acpschema.MCPServer{},
	})
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
	if err := m.store.SaveMessages(session.ID, []provider.Message{provider.UserMessage(req.Message)}); err != nil {
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
		State:      StateStarting,
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
		toolByID:   make(map[string]ToolCallSnapshot),
	}
	m.addJob(job, conn, peer, cancel)
	job.startTurn()
	go m.runPrompt(runCtx, job, req.Message)

	return SpawnResult{
		Status:    "accepted",
		SessionID: job.ID,
		Slug:      job.Slug,
		ACPAgent:  job.ACPAgent,
		State:     StateRunning,
	}, nil
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
	messages, _ := m.store.LoadMessages(job.ID)
	messages = append(messages, provider.UserMessage(req.Message))
	_ = m.store.SaveMessages(job.ID, messages)
	job.startTurn()
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

func (m *Manager) runPrompt(ctx context.Context, job *Job, message string) {
	job.turnMu.Lock()
	defer job.turnMu.Unlock()

	job.mu.RLock()
	done := job.done
	job.mu.RUnlock()
	if done == nil {
		done = job.startTurn()
	}

	peer := m.peer(job.ID)
	if peer == nil {
		job.setState(StateFailed, "", "acp peer is not active")
		close(done)
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
		close(done)
		return
	}
	var resp struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		job.setState(StateFailed, "", err.Error())
		close(done)
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
	close(done)
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
	messages, err := m.store.LoadMessages(sessionID)
	if err != nil {
		return
	}
	messages = append(messages, provider.AssistantMessage(content, nil))
	_ = m.store.SaveMessages(sessionID, messages)
}

func (m *Manager) handleJSONRPC(ctx context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
	switch req.Method {
	case acpschema.ClientMethodSessionUpdate:
		var note struct {
			SessionID string          `json:"sessionId"`
			Update    json.RawMessage `json:"update"`
		}
		if err := json.Unmarshal(req.Params, &note); err != nil {
			return nil, jsonrpc.InvalidParams("invalid session/update", map[string]any{"error": err.Error()})
		}
		m.applyUpdate(note.SessionID, note.Update)
		return jsonrpc.EncodeResult(map[string]any{})
	case acpschema.ClientMethodSessionRequestPermission:
		return jsonrpc.EncodeResult(map[string]any{"outcome": "cancelled"})
	case acpschema.ClientMethodFSReadTextFile:
		return m.readTextFile(req.Params)
	case acpschema.ClientMethodFSWriteTextFile:
		return m.writeTextFile(req.Params)
	case acpschema.ClientMethodTerminalKill, acpschema.ClientMethodTerminalRelease:
		return jsonrpc.EncodeResult(map[string]any{})
	case acpschema.ClientMethodTerminalCreate, acpschema.ClientMethodTerminalOutput, acpschema.ClientMethodTerminalWaitForExit:
		return nil, jsonrpc.InternalError("terminal support is disabled", nil)
	default:
		return nil, jsonrpc.MethodNotFound(req.Method)
	}
}

func (m *Manager) readTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.ReadTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/read_text_file", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := safePath(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	content := string(data)
	if req.Limit > 0 && len(content) > req.Limit {
		content = content[:req.Limit]
	}
	return jsonrpc.EncodeResult(acpschema.ReadTextFileResponse{Content: content})
}

func (m *Manager) writeTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.WriteTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/write_text_file", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := safePath(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	return jsonrpc.EncodeResult(acpschema.WriteTextFileResponse{})
}

func (m *Manager) applyUpdate(acpSessionID string, raw json.RawMessage) {
	job := m.jobByACP(acpSessionID)
	if job == nil {
		return
	}
	var env struct {
		SessionUpdate string          `json:"sessionUpdate"`
		Content       json.RawMessage `json:"content"`
		Title         string          `json:"title"`
		ToolCallID    string          `json:"toolCallId"`
		Status        json.RawMessage `json:"status"`
		Entries       []struct {
			Content  string          `json:"content"`
			Status   json.RawMessage `json:"status"`
			Priority json.RawMessage `json:"priority"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	switch env.SessionUpdate {
	case "agent_message_chunk":
		job.Assistant += contentText(env.Content)
	case "agent_thought_chunk":
		job.Thought += contentText(env.Content)
	case "tool_call", "tool_call_update":
		call := job.toolByID[env.ToolCallID]
		call.ID = env.ToolCallID
		if env.Title != "" {
			call.Title = env.Title
		}
		if len(env.Status) > 0 {
			call.Status = rawString(env.Status)
		}
		job.toolByID[env.ToolCallID] = call
		job.ToolCalls = sortedToolCalls(job.toolByID)
	case "plan":
		job.Plan = make([]PlanEntry, 0, len(env.Entries))
		for _, entry := range env.Entries {
			job.Plan = append(job.Plan, PlanEntry{
				Content:  entry.Content,
				Status:   rawString(entry.Status),
				Priority: rawString(entry.Priority),
			})
		}
	case "session_info_update":
		if env.Title != "" {
			job.Title = env.Title
			if session, err := m.store.LoadSession(job.ID); err == nil {
				session.Title = env.Title
				_ = m.store.SaveSession(session)
			}
		}
	}
	job.UpdatedAt = time.Now().UTC()
}

func (m *Manager) openConn(ctx context.Context, name string, cfg AgentConfig) (jsonrpc.MessageConn, error) {
	if cfg.URL != "" {
		opts := []streamhttp.ClientOption{}
		parsed, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, err
		}
		if parsed.Scheme == "http" {
			opts = append(opts, streamhttp.WithH2C())
		}
		if cfg.Token != "" {
			opts = append(opts, streamhttp.WithBearerToken(cfg.Token))
		}
		return streamhttp.Dial(cfg.URL, opts...)
	}
	command := cfg.Command
	if command == "" {
		command = defaultCommand(name)
	}
	if command == "" {
		return nil, fmt.Errorf("acp agent %q has no command", name)
	}
	cmd := exec.CommandContext(ctx, command, cfg.Args...)
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	conn := stdio.New(stdout, stdin)
	go func() {
		_ = cmd.Wait()
		_ = conn.Close()
	}()
	return conn, nil
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

func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:         j.ID,
		Slug:       j.Slug,
		Title:      j.Title,
		ParentID:   j.ParentID,
		ACPAgent:   j.ACPAgent,
		ACPSession: j.ACPSession,
		Cwd:        j.Cwd,
		State:      j.State,
		StopReason: j.StopReason,
		Assistant:  j.Assistant,
		Thought:    j.Thought,
		Plan:       append([]PlanEntry(nil), j.Plan...),
		ToolCalls:  append([]ToolCallSnapshot(nil), j.ToolCalls...),
		Error:      j.Error,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
	}
}

func (j *Job) setState(state, stopReason, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = state
	j.StopReason = stopReason
	j.Error = errMsg
	j.UpdatedAt = time.Now().UTC()
}

func (j *Job) startTurn() chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.State = StateRunning
	j.Error = ""
	j.StopReason = ""
	j.done = make(chan struct{})
	j.UpdatedAt = time.Now().UTC()
	return j.done
}

func autoAuthMethod(raw json.RawMessage) string {
	var init struct {
		AuthMethods []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Vars []struct {
				Name string `json:"name"`
			} `json:"vars"`
		} `json:"authMethods"`
	}
	if err := json.Unmarshal(raw, &init); err != nil {
		return ""
	}
	for _, method := range init.AuthMethods {
		if method.Type != "env_var" || len(method.Vars) == 0 {
			continue
		}
		allSet := true
		for _, v := range method.Vars {
			if os.Getenv(v.Name) == "" {
				allSet = false
				break
			}
		}
		if allSet {
			return method.ID
		}
	}
	return ""
}

func contentText(raw json.RawMessage) string {
	var block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &block); err == nil && block.Type == "text" {
		return block.Text
	}
	return ""
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func sortedToolCalls(in map[string]ToolCallSnapshot) []ToolCallSnapshot {
	out := make([]ToolCallSnapshot, 0, len(in))
	for _, call := range in {
		out = append(out, call)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func safePath(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		name = strings.TrimPrefix(filepath.Clean(name), string(filepath.Separator))
	}
	path := filepath.Join(root, name)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", name)
	}
	return abs, nil
}

func defaultCommand(name string) string {
	switch name {
	case "codex":
		return "codex-acp"
	case "claude_code":
		return "claude-code-acp"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
