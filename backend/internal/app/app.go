package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/jazagent"
	"github.com/wins/jaz/backend/internal/loops"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/runtimeenv"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/templates/acpcompletion"
	"github.com/wins/jaz/backend/internal/tools"
	agentcancel "github.com/wins/jaz/backend/internal/tools/agent/cancel"
	agentlist "github.com/wins/jaz/backend/internal/tools/agent/list"
	agentsend "github.com/wins/jaz/backend/internal/tools/agent/send"
	agentspawn "github.com/wins/jaz/backend/internal/tools/agent/spawn"
	agentstatus "github.com/wins/jaz/backend/internal/tools/agent/status"
	agentwait "github.com/wins/jaz/backend/internal/tools/agent/wait"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	plantool "github.com/wins/jaz/backend/internal/tools/plan"
	viewimagetool "github.com/wins/jaz/backend/internal/tools/viewimage"
	visualizetool "github.com/wins/jaz/backend/internal/tools/visualize"
	widgettool "github.com/wins/jaz/backend/internal/tools/widget"
	"github.com/wins/jaz/backend/internal/voice"
	mistralvoice "github.com/wins/jaz/backend/internal/voice/mistral"
	"github.com/wins/jaz/backend/internal/widgets"
	"go.uber.org/fx"
)

type Config struct {
	Root           string
	Workspace      string
	ModelProviders map[string]provider.ModelProviderConfig
	Voice          VoiceConfig
	ACP            acp.Config
	Memory         MemoryConfig
}

type VoiceConfig struct {
	TTS     SpeechConfig
	STT     SpeechConfig
	Mistral MistralConfig
}

type SpeechConfig struct {
	Provider string
	Model    string
	Voice    string
}

type MistralConfig struct {
	APIKey  string
	BaseURL string
}

type MemoryConfig struct {
	Root      string
	DBPath    string
	Scheduler bool
}

type Workspace string

type Stores struct {
	fx.Out

	Store        *sqlitestore.Store
	ACPStore     acp.Store
	StorageStore storage.Store
}

func NewRuntimeLayout(cfg Config) (runtimefiles.Layout, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		root = sqlitestore.DefaultRoot()
	}
	layout, err := runtimefiles.Ensure(root)
	if err != nil {
		return runtimefiles.Layout{}, err
	}
	if err := skills.InstallDefaults(layout.Root); err != nil {
		return runtimefiles.Layout{}, err
	}
	return layout, nil
}

func NewStore(layout runtimefiles.Layout, catalog acp.AgentCatalog) (Stores, error) {
	store, err := sqlitestore.New(layout.Root)
	if err != nil {
		return Stores{}, err
	}
	if err := agentsettings.EnsureAgentDefaults(store, agentDefaultsSeed(catalog)); err != nil {
		_ = store.Close()
		return Stores{}, err
	}
	return Stores{Store: store, ACPStore: store, StorageStore: store}, nil
}

func NewWorkspace(cfg Config, layout runtimefiles.Layout) (Workspace, error) {
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = layout.DefaultWorkspace
	}
	return Workspace(workspace), os.MkdirAll(workspace, 0o755)
}

func NewMemory(cfg Config, layout runtimefiles.Layout) (*jazmem.Memory, error) {
	memoryRoot := strings.TrimSpace(cfg.Memory.Root)
	dbPath := strings.TrimSpace(cfg.Memory.DBPath)
	if memoryRoot == "" {
		memoryRoot = filepath.Join(layout.Root, "memory")
		if dbPath == "" {
			dbPath = filepath.Join(layout.Root, "jazmem.sqlite")
		}
	}
	return jazmem.Open(jazmem.Config{Root: memoryRoot, DBPath: dbPath})
}

func NewPromptBuilder(store *sqlitestore.Store, workspace Workspace, memory *memoryservice.Service) *coordinator.Builder {
	return coordinator.NewBuilder(store.RootDir(), string(workspace), memory.Root(), memory.Enabled)
}

func NewACPAgentCatalog(cfg Config) acp.AgentCatalog {
	return acp.MergeAgents(acp.MergeAgents(acp.BuiltinAgents(), jazagent.ACPAgentCatalog()), cfg.ACP.Agents)
}

func NewACPAgentConfigSource(store *sqlitestore.Store, catalog acp.AgentCatalog) acp.AgentConfigSource {
	return agentsettings.NewACPConfigSource(store, catalog)
}

func NewACPConfig(cfg Config, store *sqlitestore.Store, workspace Workspace, prompts *coordinator.Builder, catalog acp.AgentCatalog, source acp.AgentConfigSource, mcpServers mcpconfig.ServerReader) acp.Config {
	cfg.ACP.Agents = catalog
	cfg.ACP.AgentSource = source
	cfg.ACP.Root = store.RootDir()
	cfg.ACP.Workspace = string(workspace)
	cfg.ACP.Providers = cfg.ModelProviders
	cfg.ACP.SystemPrompt = prompts
	cfg.ACP.MCPStore = mcpServers
	cfg.ACP.MCPTokens = store
	return cfg.ACP
}

func agentDefaultsSeed(catalog acp.AgentCatalog) agentsettings.AgentDefaults {
	return agentsettings.AgentDefaultsFromCatalog(catalog)
}

func NewWidgetService(store *sqlitestore.Store, logger *log.Logger) *widgets.Service {
	return widgets.NewService(store, logger)
}

func NewWidgetSessionPublisher(service *widgets.Service, store *sqlitestore.Store) *widgets.SessionPublisher {
	return &widgets.SessionPublisher{Service: service, Sessions: store, Loops: store}
}

func NewToolRegistry(commandManager *exectool.CommandManager, workspace Workspace, manager *acp.Manager, store *sqlitestore.Store, events *sessionevents.Bus, widgetPublisher *widgets.SessionPublisher) *tools.Registry {
	return tools.NewRegistry(
		&plantool.Tool{Store: store, Events: events},
		&exectool.ExecCommandTool{Manager: commandManager, Workspace: string(workspace)},
		&exectool.WriteStdinTool{Manager: commandManager},
		&applypatch.Tool{
			Workspace:  string(workspace),
			ExtraRoots: []string{loops.AutomationsDir(store.RootDir())},
			PathScope:  applypatch.AbsolutePaths,
		},
		&visualizetool.ReadMeTool{},
		&visualizetool.ShowWidgetTool{},
		&viewimagetool.Tool{Workspace: string(workspace)},
		&widgettool.Tool{Publisher: widgetPublisher},
		&agentspawn.Tool{Manager: manager},
		&agentsend.Tool{Manager: manager},
		&agentstatus.Tool{Manager: manager},
		&agentwait.Tool{Manager: manager},
		&agentcancel.Tool{Manager: manager},
		&agentlist.Tool{Manager: manager},
	)
}

func StartMCPManager(lc fx.Lifecycle, manager *mcpruntime.Manager, logger *log.Logger) {
	if manager == nil {
		return
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		// Connecting to MCP servers must never block or fail startup: a slow or
		// unreachable server would otherwise blow the lifecycle start deadline and
		// take the whole backend down. Refresh in the background on its own context
		// so the server comes up immediately regardless of MCP connectivity.
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			manager.RefreshLocal(ctx)
			go manager.Refresh(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			manager.Close()
			return nil
		},
	})
}

func CloseMemory(lc fx.Lifecycle, memory *jazmem.Memory) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return memory.Close()
		},
	})
}

func StartMemoryScheduler(lc fx.Lifecycle, memory *memoryservice.Service) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if memory.Enabled() {
				memory.Scheduler.Start()
			}
			return nil
		},
		OnStop: func(context.Context) error {
			memory.Scheduler.Stop()
			return nil
		},
	})
}

func NewProvider(cfg Config) (provider.Provider, error) {
	reloadable := &ReloadableProvider{
		cfg:     cfg,
		envPath: runtimeenv.Path(runtimeRoot(cfg.Root)),
	}
	return reloadable, reloadable.Reload()
}

type ReloadableProvider struct {
	mu      sync.RWMutex
	cfg     Config
	envPath string
	current provider.Provider
}

func (p *ReloadableProvider) Reload() error {
	next := buildProvider(p.cfg, p.envPath)
	p.mu.Lock()
	p.current = next
	p.mu.Unlock()
	return nil
}

func (p *ReloadableProvider) APIKeyConfigured(id string) bool {
	cfg := providerConfigWithRuntimeEnv(p.cfg.ModelProviders[id], id, p.envPath)
	return strings.TrimSpace(cfg.APIKey) != ""
}

func (p *ReloadableProvider) APIKeyEnvPath() string {
	return p.envPath
}

func (p *ReloadableProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	current := p.currentProvider()
	return current.Complete(ctx, req)
}

func (p *ReloadableProvider) StreamComplete(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	current := p.currentProvider()
	return current.StreamComplete(ctx, req)
}

func (p *ReloadableProvider) currentProvider() provider.Provider {
	p.mu.RLock()
	current := p.current
	p.mu.RUnlock()
	if current == nil {
		return provider.UnavailableProvider{ID: "native", Reason: "provider has not loaded"}
	}
	return current
}

func buildProvider(cfg Config, envPath string) provider.Provider {
	clients := map[string]provider.Provider{}
	for _, meta := range provider.NativeProviders() {
		id := meta.ID
		config := providerConfigWithRuntimeEnv(cfg.ModelProviders[id], id, envPath)
		config.Type = id
		if strings.TrimSpace(config.BaseURL) == "" {
			config.BaseURL = meta.BaseURL
		}
		client, err := nativeProviderClient(id, config)
		if err != nil {
			clients[id] = provider.UnavailableProvider{ID: id, Reason: err.Error()}
			continue
		}
		clients[id] = client
	}
	if _, ok := cfg.ModelProviders[provider.ProviderMock]; ok {
		clients[provider.ProviderMock] = mockprovider.New()
	}
	return provider.NewRouter("", clients)
}

func providerConfigWithRuntimeEnv(cfg provider.ModelProviderConfig, id, envPath string) provider.ModelProviderConfig {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return cfg
	}
	meta, ok := provider.NativeProviderByID(id)
	if !ok || strings.TrimSpace(meta.APIKeyEnv) == "" {
		return cfg
	}
	if value, ok := runtimeenv.Lookup(envPath, meta.APIKeyEnv); ok {
		cfg.APIKey = value
	}
	return cfg
}

func runtimeRoot(root string) string {
	root = strings.TrimSpace(root)
	if root != "" {
		return root
	}
	return sqlitestore.DefaultRoot()
}

func nativeProviderClient(id string, cfg provider.ModelProviderConfig) (provider.Provider, error) {
	switch id {
	case provider.ProviderOpenAI, provider.ProviderOpenRouter:
		if cfg.APIKey == "" {
			meta, _ := provider.NativeProviderByID(id)
			keyEnv := meta.APIKeyEnv
			if keyEnv == "" {
				keyEnv = strings.ToUpper(id) + "_API_KEY"
			}
			return nil, fmt.Errorf("set %s or %s.apikey", keyEnv, id)
		}
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			meta, _ := provider.NativeProviderByID(id)
			baseURL = meta.BaseURL
		}
		return openaiprovider.New(baseURL, cfg.APIKey, ""), nil
	case provider.ProviderMock:
		return mockprovider.New(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q; valid providers are openai, openrouter, mock", id)
	}
}

type VoiceProviders struct {
	fx.Out

	STT voice.STT
	TTS voice.TTS
}

// NewVoice returns nil providers (voice endpoints disabled) when the selected
// provider has no API key configured, rather than failing startup.
func NewVoice(cfg Config) (VoiceProviders, error) {
	stt, err := newSTT(cfg.Voice)
	if err != nil {
		return VoiceProviders{}, err
	}
	tts, err := newTTS(cfg.Voice)
	if err != nil {
		return VoiceProviders{}, err
	}
	return VoiceProviders{STT: stt, TTS: tts}, nil
}

func newSTT(cfg VoiceConfig) (voice.STT, error) {
	switch strings.ToLower(cfg.STT.Provider) {
	case "", "mistral":
		if cfg.Mistral.APIKey == "" {
			return nil, nil
		}
		return mistralvoice.New(mistralvoice.Config{
			APIKey:   cfg.Mistral.APIKey,
			BaseURL:  cfg.Mistral.BaseURL,
			STTModel: cfg.STT.Model,
		}), nil
	default:
		return nil, fmt.Errorf("unknown stt provider %q; valid providers are mistral", cfg.STT.Provider)
	}
}

func newTTS(cfg VoiceConfig) (voice.TTS, error) {
	switch strings.ToLower(cfg.TTS.Provider) {
	case "", "mistral":
		if cfg.Mistral.APIKey == "" {
			return nil, nil
		}
		return mistralvoice.New(mistralvoice.Config{
			APIKey:   cfg.Mistral.APIKey,
			BaseURL:  cfg.Mistral.BaseURL,
			TTSModel: cfg.TTS.Model,
			Voice:    cfg.TTS.Voice,
		}), nil
	default:
		return nil, fmt.Errorf("unknown tts provider %q; valid providers are mistral", cfg.TTS.Provider)
	}
}

func NewAgent(cfg Config, modelProvider provider.Provider, registry *tools.Registry) *agent.Agent {
	return &agent.Agent{
		Provider: modelProvider,
		Tools:    registry,
		MaxTurns: agent.DefaultMaxTurns,
	}
}

func ConnectLocalJazAgent(manager *acp.Manager, a *agent.Agent, store *sqlitestore.Store, prompts *coordinator.Builder, logger *log.Logger) {
	jazagent.RegisterACP(manager, jazagent.ACPDependencies{
		Agent:   a,
		Store:   store,
		Prompts: prompts,
		Log:     logger.WithPrefix("jazagent"),
	})
}

func ConnectACPCompletion(manager *acp.Manager, a *agent.Agent, store *sqlitestore.Store, locks *sessionlock.Locks, events *sessionevents.Bus, prompts *coordinator.Builder, logger *log.Logger) {
	manager.Events = events
	manager.Done = func(ctx context.Context, job acp.Job) {
		completeACP(ctx, a, store, locks, events, prompts, logger.WithPrefix("coordinator"), job)
	}
}

func completeACP(ctx context.Context, a *agent.Agent, store *sqlitestore.Store, locks *sessionlock.Locks, events *sessionevents.Bus, prompts *coordinator.Builder, logger *log.Logger, job acp.Job) {
	if job.ParentID == "" {
		return
	}
	logger.Info("acp child finished, running follow-up", "child", job.ID, "parent", job.ParentID, "state", job.State)
	unlock := locks.Lock(job.ParentID)
	defer unlock()

	parent, err := store.LoadSession(job.ParentID)
	if err != nil {
		logger.Error("loading parent session failed", "parent", job.ParentID, "error", err)
		setStoredSessionError(store, job.ParentID, err.Error())
		return
	}
	messages, err := store.LoadMessages(job.ParentID)
	if err != nil {
		logger.Error("loading parent messages failed", "parent", job.ParentID, "error", err)
		setStoredSessionError(store, job.ParentID, err.Error())
		return
	}
	// Append-only: a full-list save would clobber rows persisted after our load.
	completion := provider.DeveloperMessage(acpCompletion(job))
	if err := store.AppendMessages(job.ParentID, completion); err != nil {
		logger.Error("appending completion note failed", "parent", job.ParentID, "error", err)
		setStoredSessionError(store, job.ParentID, err.Error())
		return
	}
	messages = append(messages, completion)
	// Same per-turn prompt rules as native turns: derive fresh, never persist
	// (older sessions stored it as their first message; skip that copy).
	for len(messages) > 0 && messages[0].OfSystem != nil {
		messages = messages[1:]
	}
	prompt, err := prompts.SystemPrompt()
	if err != nil {
		logger.Error("building system prompt failed", "parent", job.ParentID, "error", err)
		setStoredSessionError(store, job.ParentID, err.Error())
		return
	}
	if prompt := strings.TrimSpace(prompt); prompt != "" {
		messages = append([]provider.Message{provider.SystemMessage(prompt)}, messages...)
	}
	ctx = sessioncontext.WithSessionID(ctx, job.ParentID)
	result, err := a.Complete(ctx, provider.Request{
		Provider:        parent.ModelProvider,
		Model:           parent.Model,
		ReasoningEffort: parent.ReasoningEffort,
		Messages:        messages,
	})
	if err != nil {
		logger.Error("coordinator follow-up failed", "parent", job.ParentID, "error", err)
		content := fmt.Sprintf("ACP session %s finished, but coordinator follow-up failed: %v", job.Slug, err)
		_ = store.AppendMessages(job.ParentID, provider.AssistantMessage(content, nil))
		setStoredSessionError(store, job.ParentID, err.Error())
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acpEvent(job)})
		return
	}
	if len(result.Messages) > len(messages) {
		_ = store.AppendMessages(job.ParentID, result.Messages[len(messages):]...)
	}
	_ = store.AddUsage(job.ParentID, storage.Usage{
		InputTokens:           result.Usage.InputTokens,
		CachedInputTokens:     result.Usage.CachedInputTokens,
		CachedWriteTokens:     result.Usage.CachedWriteTokens,
		OutputTokens:          result.Usage.OutputTokens,
		ReasoningOutputTokens: result.Usage.ReasoningOutputTokens,
		TotalTokens:           result.Usage.TotalTokens,
	})
	if content := provider.MessageContent(result.Message); content != "" {
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acpEvent(job)})
	}
	setStoredSessionStatus(store, job.ParentID, storage.StatusIdle)
}

func setStoredSessionStatus(store *sqlitestore.Store, sessionID, status string) {
	if status == "" {
		return
	}
	session, err := store.LoadSession(sessionID)
	if err != nil {
		return
	}
	session.Status = status
	if status != storage.StatusError {
		session.Error = ""
	}
	_ = store.SaveSession(session)
}

func setStoredSessionError(store *sqlitestore.Store, sessionID, message string) {
	session, err := store.LoadSession(sessionID)
	if err != nil {
		return
	}
	session.Status = storage.StatusError
	session.Error = firstNonEmpty(message, session.Error, "Unknown error.")
	_ = store.SaveSession(session)
}

func acpCompletion(job acp.Job) string {
	prompt, err := acpcompletion.Render(acpcompletion.Data{
		Slug:      job.Slug,
		Agent:     job.ACPAgent,
		State:     job.State,
		Error:     job.Error,
		Assistant: job.Assistant,
	})
	if err != nil {
		// Embedded and parse-checked at init; the completion turn must still fire.
		return fmt.Sprintf("ACP session %s (%s) completed with state %s. Report the outcome to the user now.", job.Slug, job.ACPAgent, job.State)
	}
	return prompt
}

func acpEvent(job acp.Job) *sessionevents.ACPEvent {
	modes := sessionevents.ACPModeState{
		CurrentModeID:   job.Modes.CurrentModeID,
		ExecutionModeID: job.Modes.ExecutionModeID,
		PlanModeID:      job.Modes.PlanModeID,
		AvailableModes:  make([]sessionevents.ACPMode, 0, len(job.Modes.AvailableModes)),
	}
	for _, mode := range job.Modes.AvailableModes {
		modes.AvailableModes = append(modes.AvailableModes, sessionevents.ACPMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	plan := make([]sessionevents.ACPPlanEntry, 0, len(job.Plan))
	for _, entry := range job.Plan {
		plan = append(plan, sessionevents.ACPPlanEntry{Content: entry.Content, Status: entry.Status, Priority: entry.Priority})
	}
	calls := make([]sessionevents.ACPToolCall, 0, len(job.ToolCalls))
	for _, call := range job.ToolCalls {
		calls = append(calls, sessionevents.ACPToolCall{ID: call.ID, Title: call.Title, Status: call.Status})
	}
	permissions := make([]sessionevents.ACPPermission, 0, len(job.Permissions))
	for _, permission := range job.Permissions {
		permission.Options = append([]sessionevents.ACPPermissionOption(nil), permission.Options...)
		permission.Locations = append([]sessionevents.ACPPermissionLocation(nil), permission.Locations...)
		if len(permission.Questions) > 0 {
			questions := make([]sessionevents.ACPQuestion, 0, len(permission.Questions))
			for _, question := range permission.Questions {
				question.Options = append([]sessionevents.ACPQuestionOption(nil), question.Options...)
				questions = append(questions, question)
			}
			permission.Questions = questions
		}
		permissions = append(permissions, permission)
	}
	return &sessionevents.ACPEvent{
		ID:          job.ID,
		Slug:        job.Slug,
		Title:       job.Title,
		ParentID:    job.ParentID,
		Agent:       job.ACPAgent,
		SessionID:   job.ACPSession,
		State:       job.State,
		StopReason:  job.StopReason,
		Assistant:   job.Assistant,
		Thought:     job.Thought,
		Error:       job.Error,
		Modes:       modes,
		Plan:        plan,
		ToolCalls:   calls,
		Permissions: permissions,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
