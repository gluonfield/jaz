package memorysearch

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/memorysearchprompt"
)

type Manager interface {
	RunWorker(context.Context, acp.SpawnRequest, string) (acp.Job, error)
}

type Runner struct {
	store   storage.SettingsStorage
	manager Manager
}

func New(store storage.SettingsStorage, manager Manager) *Runner {
	return &Runner{store: store, manager: manager}
}

func (r *Runner) SearchMemory(ctx context.Context, req memoryservice.SearchRequest) (string, error) {
	config, err := settings.LoadMemorySettings(r.store)
	if err != nil {
		return "", err
	}
	if !config.Enabled {
		return "", errors.New("memory is disabled in settings")
	}
	if config.Agent == "" || acp.CanonicalAgentName(config.Agent) == acp.AgentJaz {
		return "", errors.New("choose a coding agent in Memory settings to search memory")
	}
	defaults, err := settings.LoadAgentDefaults(r.store)
	if errors.Is(err, storage.ErrSettingNotFound) {
		defaults = settings.DefaultAgentDefaults()
	} else if err != nil {
		return "", err
	}
	prompt, err := memorysearchprompt.Render(memorysearchprompt.Data{
		Query: req.Query,
		Limit: req.Limit,
		Deep:  req.Deep,
	})
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	job, err := r.manager.RunWorker(ctx, acp.SpawnRequest{
		ParentID:        req.ParentID,
		ACPAgent:        config.Agent,
		Slug:            "memory-search-" + id,
		Title:           "Memory Search",
		Directory:       ".jaz-runtime/memory-search",
		Model:           config.WorkerModel(defaults),
		ReasoningEffort: config.WorkerReasoningEffort(defaults),
		SourceType:      storage.SourceMemorySearch,
		SourceID:        id,
	}, prompt)
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(job.Assistant)
	if answer == "" {
		return "", errors.New("memory search returned an empty answer")
	}
	return answer, nil
}
