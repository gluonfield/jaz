package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/provider"
	mockprovider "github.com/wins/jaz/backend/internal/provider/mock"
	openaiprovider "github.com/wins/jaz/backend/internal/provider/openai"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
	agentcancel "github.com/wins/jaz/backend/internal/tools/agent/cancel"
	agentlist "github.com/wins/jaz/backend/internal/tools/agent/list"
	agentsend "github.com/wins/jaz/backend/internal/tools/agent/send"
	agentspawn "github.com/wins/jaz/backend/internal/tools/agent/spawn"
	agentstatus "github.com/wins/jaz/backend/internal/tools/agent/status"
	agentwait "github.com/wins/jaz/backend/internal/tools/agent/wait"
	applypatch "github.com/wins/jaz/backend/internal/tools/applypatch"
	exectool "github.com/wins/jaz/backend/internal/tools/exec"
	"github.com/wins/jaz/backend/internal/voice"
	mistralvoice "github.com/wins/jaz/backend/internal/voice/mistral"
	"go.uber.org/fx"
)

type Config struct {
	Root      string
	Workspace string
	Provider  ProviderConfig
	Voice     VoiceConfig
	ACP       acp.Config
}

type ProviderConfig struct {
	Type   string
	APIKey string
	Model  string
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

type Workspace string

type Stores struct {
	fx.Out

	Store        *sqlitestore.Store
	ACPStore     acp.Store
	StorageStore storage.Store
}

func NewStore(cfg Config) (Stores, error) {
	store, err := sqlitestore.New(cfg.Root)
	if err != nil {
		return Stores{}, err
	}
	return Stores{Store: store, ACPStore: store, StorageStore: store}, nil
}

func NewWorkspace(cfg Config, store *sqlitestore.Store) (Workspace, error) {
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = store.DefaultWorkspace()
	}
	return Workspace(workspace), os.MkdirAll(workspace, 0o755)
}

func NewPromptBuilder(store *sqlitestore.Store, logger *log.Logger) *coordinator.Builder {
	return coordinator.NewBuilder(store.RootDir(), logger.WithPrefix("prompt"))
}

func NewACPConfig(cfg Config, store *sqlitestore.Store, workspace Workspace, prompts *coordinator.Builder) acp.Config {
	cfg.ACP.Root = store.RootDir()
	cfg.ACP.Workspace = string(workspace)
	cfg.ACP.SystemPrompt = prompts
	return cfg.ACP
}

func NewToolRegistry(commandManager *exectool.CommandManager, workspace Workspace, manager *acp.Manager) *tools.Registry {
	return tools.NewRegistry(
		&exectool.ExecCommandTool{Manager: commandManager, Workspace: string(workspace)},
		&exectool.WriteStdinTool{Manager: commandManager},
		&applypatch.Tool{Workspace: string(workspace)},
		&agentspawn.Tool{Manager: manager},
		&agentsend.Tool{Manager: manager},
		&agentstatus.Tool{Manager: manager},
		&agentwait.Tool{Manager: manager},
		&agentcancel.Tool{Manager: manager},
		&agentlist.Tool{Manager: manager},
	)
}

func NewProvider(cfg Config) (provider.Provider, error) {
	switch strings.ToLower(cfg.Provider.Type) {
	case "", "openai":
		if cfg.Provider.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires an API key; set OPENAI_API_KEY or openai.apikey")
		}
		return openaiprovider.New("https://api.openai.com/v1", cfg.Provider.APIKey, cfg.Provider.Model), nil
	case "openrouter":
		if cfg.Provider.APIKey == "" {
			return nil, fmt.Errorf("openrouter provider requires an API key; set OPENROUTER_API_KEY or openrouter.apikey")
		}
		return openaiprovider.New("https://openrouter.ai/api/v1", cfg.Provider.APIKey, cfg.Provider.Model), nil
	case "mock":
		return mockprovider.New(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q; valid providers are openai, openrouter, mock", cfg.Provider.Type)
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
		Model:    cfg.Provider.Model,
		Tools:    registry,
		MaxTurns: agent.DefaultMaxTurns,
	}
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
	if prompt := strings.TrimSpace(prompts.SystemPrompt()); prompt != "" {
		messages = append([]provider.Message{provider.SystemMessage(prompt)}, messages...)
	}
	ctx = sessioncontext.WithSessionID(ctx, job.ParentID)
	result, err := a.Complete(ctx, provider.Request{Messages: messages})
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
	var b strings.Builder
	fmt.Fprintf(&b, "ACP session %s (%s) completed with state %s.", job.Slug, job.ACPAgent, job.State)
	if job.Error != "" {
		fmt.Fprintf(&b, "\nError: %s", job.Error)
	}
	if job.Assistant != "" {
		fmt.Fprintf(&b, "\nAssistant result:\n%s", job.Assistant)
	}
	b.WriteString("\n\nReport the outcome to the user now: what was delivered, where it lives (files, paths, URLs), and how it was verified. Be concrete and base it only on the result above; do not say work is still in progress.")
	return b.String()
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
