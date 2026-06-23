package memorysearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memoryservice"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/memorysearchprompt"
)

const (
	Timeout         = 90 * time.Second
	workerDirectory = ".jaz-runtime/memory-search"
)

type Manager interface {
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Wait(context.Context, acp.WaitRequest) (acp.Job, error)
	Cancel(context.Context, string) (acp.Job, error)
}

type Runner struct {
	Store   storage.SettingsStorage
	Manager Manager
	Now     func() time.Time
}

func New(store storage.SettingsStorage, manager Manager) *Runner {
	return &Runner{Store: store, Manager: manager}
}

func (r *Runner) SearchMemory(ctx context.Context, req memoryservice.AgenticSearchRequest) (string, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	settings, err := jazsettings.LoadMemorySettings(r.Store)
	if err != nil {
		return "", err
	}
	agent := acp.CanonicalAgentName(settings.Agent)
	if agent == "" {
		return "", fmt.Errorf("memory agent is not configured")
	}
	if agent == acp.AgentJaz {
		return "", fmt.Errorf("built-in Jaz cannot be used as the memory agent yet")
	}
	agentDefaults, err := jazsettings.LoadAgentDefaults(r.Store)
	if errors.Is(err, storage.ErrSettingNotFound) {
		agentDefaults = jazsettings.DefaultAgentDefaults()
	} else if err != nil {
		return "", err
	}
	stamp := r.now().UnixNano()
	spawned, err := r.Manager.Spawn(ctx, acp.SpawnRequest{
		ParentID:        strings.TrimSpace(req.ParentID),
		ACPAgent:        agent,
		Slug:            fmt.Sprintf("memory-search-%s-%d", agent, stamp),
		Title:           "Memory Search",
		Directory:       workerDirectory,
		Model:           jazsettings.WorkerAgentModel(agent, agentDefaults),
		ReasoningEffort: jazsettings.WorkerAgentReasoningEffort(agent),
		SourceType:      storage.SourceMemorySearch,
		SourceID:        fmt.Sprintf("%d", stamp),
		MCPServerPolicy: acp.MCPServerPolicyMemorySearchWorker,
	})
	if err != nil {
		return "", err
	}
	prompt, err := memorysearchprompt.Render(memorysearchprompt.Data{Query: query, Deep: req.Deep})
	if err != nil {
		r.cancelWorker(spawned.SessionID)
		return "", err
	}
	if _, err := r.Manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    prompt,
		Completion: acp.CompletionInline,
	}); err != nil {
		r.cancelWorker(spawned.SessionID)
		return "", err
	}
	job, err := r.Manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: Timeout})
	if err != nil {
		r.cancelWorker(spawned.SessionID)
		return "", err
	}
	if job.State == acp.StateRunning || job.State == acp.StateStarting {
		r.cancelWorker(spawned.SessionID)
		return "", fmt.Errorf("memory search timed out after %s", Timeout)
	}
	if job.State != acp.StateIdle {
		if strings.TrimSpace(job.Error) != "" {
			return "", fmt.Errorf("memory search failed: %s", job.Error)
		}
		return "", fmt.Errorf("memory search finished with state %q", job.State)
	}
	answer := strings.TrimSpace(job.Assistant)
	if answer == "" {
		return "", fmt.Errorf("memory search returned an empty answer")
	}
	return answer, nil
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) cancelWorker(sessionID string) {
	cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = r.Manager.Cancel(cancelCtx, sessionID)
}
