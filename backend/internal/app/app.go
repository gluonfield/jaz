package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpadapter"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/browsertask"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/jazagent"
	"github.com/wins/jaz/backend/internal/loops"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/memorysearch"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/providerstore"
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
	"github.com/wins/jaz/backend/internal/threads"
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
	Skills         SkillsConfig
	Voice          VoiceConfig
	ACP            acp.Config
	Memory         MemoryConfig
	Connections    ConnectionsConfig
}

type Release struct {
	Version string
}

type SkillsConfig struct {
	DisableSync bool
}

const (
	defaultSkillsManifestURL    = "https://github.com/gluonfield/jaz-skills/releases/download/jaz-v0.0.28/manifest.json"
	defaultSkillsManifestSHA256 = "7acc2b360b5955721ae38edda1b75f5eb4409676a45dfad603e7325c3eab7497"
)

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

type ConnectionsConfig struct {
	Gmail GmailConnectionConfig
}

type GmailConnectionConfig struct {
	OAuthClientID     string
	OAuthClientSecret string
}

type Workspace string

type Stores struct {
	fx.Out

	Store           *sqlitestore.Store
	ACPStore        acp.Store
	StorageStore    storage.Store
	SessionStore    storage.SessionStore
	SessionEvents   storage.SessionEventAppender
	UsageEventStore storage.UsageEventStore
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
	return Stores{
		Store:           store,
		ACPStore:        store,
		StorageStore:    store,
		SessionStore:    store,
		SessionEvents:   store,
		UsageEventStore: store,
	}, nil
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

func NewACPAdapterManager(layout runtimefiles.Layout, release Release) *acpadapter.Manager {
	return acpadapter.New(layout.Root, release.Version)
}

func StartACPAdapterDownloads(lc fx.Lifecycle, catalog acp.AgentCatalog, adapters *acpadapter.Manager, logger *log.Logger) {
	if adapters == nil {
		return
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			for _, adapter := range managedAdapterNames(catalog) {
				go func(adapter string) {
					if err := adapters.Prepare(ctx, adapter); err != nil && ctx.Err() == nil {
						logger.WithPrefix("acp-adapter").Warn("managed adapter download failed", "adapter", adapter, "error", err)
					}
				}(adapter)
			}
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

func managedAdapterNames(catalog acp.AgentCatalog) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, name := range catalog.Names() {
		cfg, _ := catalog.Agent(name)
		adapter := strings.TrimSpace(cfg.ManagedAdapter)
		if adapter == "" {
			continue
		}
		if _, ok := seen[adapter]; ok {
			continue
		}
		seen[adapter] = struct{}{}
		out = append(out, adapter)
	}
	return out
}

// NewProviderSource builds the live, thread-safe registry of effective model
// providers (application.yaml + native keys base, overlaid with DB-backed
// customs). It's the single instance the server, ACP manager, and runtime read
// through, so a runtime add/edit/delete propagates without a restart.
func NewProviderSource(cfg Config, store *sqlitestore.Store) (provider.Source, error) {
	return provider.NewSource(cfg.ModelProviders, providerstore.Loader{Store: store})
}

func NewACPConfig(cfg Config, store *sqlitestore.Store, workspace Workspace, prompts *coordinator.Builder, catalog acp.AgentCatalog, source acp.AgentConfigSource, adapters *acpadapter.Manager, mcpServers mcpconfig.ServerReader, providerSource provider.Source, widgetService *widgets.Service) acp.Config {
	cfg.ACP.Agents = catalog
	cfg.ACP.AgentSource = source
	cfg.ACP.Adapters = adapters
	cfg.ACP.Root = store.RootDir()
	cfg.ACP.Workspace = string(workspace)
	cfg.ACP.Providers = cfg.ModelProviders
	cfg.ACP.ProviderSource = providerSource
	cfg.ACP.SystemPrompt = prompts
	cfg.ACP.MCPStore = mcpServers
	promptBuilder := loops.RuntimePromptBuilder{Repo: store}
	if widgetService != nil {
		promptBuilder.PromptExtra = widgetService.LoopPromptExtra
	}
	cfg.ACP.ResumePrompt = func(session storage.Session) (promptmodule.Modules, error) {
		switch session.SourceType {
		case storage.SourceBrowserTask:
			return promptmodule.New(browsertask.WorkerSystemPrompt()), nil
		case storage.SourceMemorySearch:
			return promptmodule.New(memorysearch.WorkerSystemPrompt()), nil
		case storage.SourceLoopRun:
			if session.SourceID == "" {
				return nil, nil
			}
			return promptBuilder.ForRun(session.SourceID, time.Now().UTC())
		default:
			return nil, nil
		}
	}
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

func NewThreadService(store *sqlitestore.Store) *threads.Service {
	return threads.NewService(sqlitestore.NewSearchQueries(store), store)
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
		&widgettool.PublishTool{Publisher: widgetPublisher},
		&viewimagetool.Tool{Workspace: string(workspace)},
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

func StartSkillSync(lc fx.Lifecycle, cfg Config, layout runtimefiles.Layout, logger *log.Logger) {
	if cfg.Skills.DisableSync {
		return
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ctx, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				syncCfg := skills.RemoteSyncConfig{
					ManifestURL:    defaultSkillsManifestURL,
					ManifestSHA256: defaultSkillsManifestSHA256,
				}
				if err := skills.SyncRemote(ctx, layout.Root, syncCfg); err != nil && ctx.Err() == nil {
					logger.WithPrefix("skills").Warn("skill sync failed", "url", defaultSkillsManifestURL, "error", err)
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
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
		return provider.UnavailableProvider{ID: "model", Reason: "provider has not loaded"}
	}
	return current
}

func buildProvider(cfg Config, envPath string) provider.Provider {
	clients := map[string]provider.Provider{}
	for _, meta := range provider.RunnableModelProviders() {
		id := meta.ID
		config := providerConfigWithRuntimeEnv(cfg.ModelProviders[id], id, envPath)
		config.Type = id
		if strings.TrimSpace(config.BaseURL) == "" {
			config.BaseURL = meta.BaseURL
		}
		client, err := modelProviderClient(id, config)
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
	meta, ok := provider.RunnableModelProviderByID(id)
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

func modelProviderClient(id string, cfg provider.ModelProviderConfig) (provider.Provider, error) {
	switch id {
	case provider.ProviderOpenAI, provider.ProviderOpenRouter:
		if cfg.APIKey == "" {
			meta, _ := provider.RunnableModelProviderByID(id)
			keyEnv := meta.APIKeyEnv
			if keyEnv == "" {
				keyEnv = strings.ToUpper(id) + "_API_KEY"
			}
			return nil, fmt.Errorf("set %s or %s.apikey", keyEnv, id)
		}
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			meta, _ := provider.RunnableModelProviderByID(id)
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
		Provider:   modelProvider,
		Tools:      registry,
		DeferTools: func(name string) bool { return registry.InGroup(mcpruntime.RegistryGroup, name) },
		MaxTurns:   agent.DefaultMaxTurns,
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
	logger.Info("acp child finished, handling parent completion", "child", job.ID, "parent", job.ParentID, "state", job.State)
	unlock := locks.Lock(job.ParentID)
	defer unlock()

	parent, err := store.LoadSession(job.ParentID)
	if err != nil {
		logger.Error("loading parent session failed", "parent", job.ParentID, "error", err)
		setStoredSessionError(store, job.ParentID, err.Error())
		return
	}
	if usesExternalACPAgent(parent) {
		content := acpParentCompletionMessage(job)
		if err := store.AppendMessages(job.ParentID, provider.AssistantMessage(content, nil)); err != nil {
			logger.Error("appending acp parent completion failed", "parent", job.ParentID, "error", err)
			setStoredSessionError(store, job.ParentID, err.Error())
			return
		}
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acp.EventFromJob(job)})
		setStoredSessionStatus(store, job.ParentID, storage.StatusIdle)
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
	// Same per-turn prompt rules as Jaz turns: derive fresh, never persist
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
		Tools:           jazagent.ToolDefinitionsForSurface(a.Tools, ""),
	})
	if err != nil {
		logger.Error("coordinator follow-up failed", "parent", job.ParentID, "error", err)
		content := fmt.Sprintf("ACP session %s finished, but coordinator follow-up failed: %v", job.Slug, err)
		_ = store.AppendMessages(job.ParentID, provider.AssistantMessage(content, nil))
		setStoredSessionError(store, job.ParentID, err.Error())
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acp.EventFromJob(job)})
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
		events.Publish(sessionevents.Event{SessionID: job.ParentID, Type: "assistant", Content: content, ACP: acp.EventFromJob(job)})
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

func usesExternalACPAgent(session storage.Session) bool {
	if session.RuntimeRef == nil {
		return false
	}
	agentName := acp.CanonicalAgentName(session.RuntimeRef.Agent)
	return agentName != "" && agentName != acp.AgentJaz
}

func acpParentCompletionMessage(job acp.Job) string {
	label := firstNonEmpty(job.Slug, job.Title, job.ID)
	state := firstNonEmpty(job.State, "finished")
	agentName := acp.CanonicalAgentName(job.ACPAgent)
	var out strings.Builder
	out.WriteString("Child session ")
	out.WriteString(label)
	if agentName != "" {
		out.WriteString(" (")
		out.WriteString(agentName)
		out.WriteString(")")
	}
	out.WriteString(" finished with state ")
	out.WriteString(state)
	out.WriteString(".")
	if job.Error != "" {
		out.WriteString("\n\nError: ")
		out.WriteString(job.Error)
	}
	if job.Assistant != "" {
		out.WriteString("\n\n")
		out.WriteString(job.Assistant)
	}
	return out.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
