package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	StopReasonCancelled      = "cancelled"
	StopReasonServerShutdown = "server_shutdown"
)

const (
	MCPServerPolicyAll                = ""
	MCPServerPolicyWidget             = "widget"
	MCPServerPolicyMemorySearchWorker = "memory_search_worker"
	MCPServerPolicyMemorySourceWorker = "memory_source_worker"
	MCPServerPolicyBrowserWorker      = "browser_worker"
)

type Store interface {
	CreateSession(storage.CreateSession) (storage.Session, error)
	LoadSession(string) (storage.Session, error)
	SaveSession(storage.Session) error
	UpdateSessionStatus(id, status, errorMessage string, attentionAt time.Time) error
	UpdateSessionTitleFromRuntime(id, title string) (storage.Session, bool, error)
	TouchSessionAttention(string) error
	storage.MessageAppender
	storage.SessionEventStore
	LoadLatestACPTurn(context.Context, string) ([]sessionevents.Event, error)
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

	mu             sync.RWMutex
	jobsByID       map[string]*jobState
	jobsBySlug     map[string]*jobState
	jobsByACP      map[string]*jobState
	processes      map[string]*agentProcess
	localAgents    map[string]LocalAgentRunner
	turnReaders    map[string]int
	pendingDiscard map[string]*jobState

	pendingPermission map[string]*pendingPermission
	permissionMu      sync.Mutex

	transcriptBuffers acpTranscriptBuffers
	projectionMu      sync.Mutex
	projections       map[string]*eventProjection

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
		processes:         make(map[string]*agentProcess),
		localAgents:       make(map[string]LocalAgentRunner),
		turnReaders:       make(map[string]int),
		pendingDiscard:    make(map[string]*jobState),
		pendingPermission: make(map[string]*pendingPermission),
		projections:       make(map[string]*eventProjection),
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
	runCtx, cancel := context.WithCancel(context.Background())
	conn, stderr, err := m.openConn(runCtx, name, cfg, env, cwd, mcpServerPolicy)
	if err != nil {
		cancel()
		return nil, err
	}
	promptTracker := newPromptTrackingConn(conn)
	peer := jsonrpc.NewPeer(promptTracker, handler)
	go func() {
		err := peer.Serve(runCtx)
		if err != nil && !errors.Is(err, context.Canceled) {
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
	if err := m.validateSpawnModelBeforePersist(ctx, req, cfg); err != nil {
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
	if err := validateProcessLifecycle(req.ACPAgent, cfg, ac.initRaw); err != nil {
		ac.close()
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
	if err := m.store.SaveSession(session); err != nil {
		ac.close()
		return fail(err)
	}
	process := newAgentProcess(ac, turnScopedAgentProcess(cfg))
	job := newIdleJob(session, req.ACPAgent, session.RuntimeRef.SessionID, absCwd, modes)
	job.promptQueueing = promptQueueingSupported(ac.initRaw)
	m.addJob(job, process)
	if process.turnScoped {
		m.closeUnusedProcess(job)
	} else {
		ac.trackPromptSends(job)
	}
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

func (m *Manager) configuredAgent(name string) (AgentConfig, bool, error) {
	return m.agents.AgentConfig(CanonicalAgentName(name))
}

// Agents lists the names of configured ACP agents for session runtimes.
func (m *Manager) Agents() []string {
	names, err := m.agents.EnabledAgentNames()
	if err != nil {
		m.log.Warn("loading configured acp agents failed", "error", err)
		return []string{}
	}
	return SelectableAgentNames(names)
}
