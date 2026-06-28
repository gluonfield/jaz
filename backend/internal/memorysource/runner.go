package memorysource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/memorysourceprompt"
)

const (
	DefaultBatchFiles = 10
	DefaultBatchChars = 100000
	Timeout           = 30 * time.Minute
)

type Manager interface {
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Wait(context.Context, acp.WaitRequest) (acp.Job, error)
	Cancel(context.Context, string) (acp.Job, error)
}

type Store interface {
	storage.SettingsStorage
}

type Runner struct {
	Root       string
	Store      Store
	Queue      *sourcequeue.Queue
	Manager    Manager
	Log        *log.Logger
	BatchFiles int
	BatchChars int
}

type batchSource struct {
	Path      string
	PendingAt time.Time
	Content   string
	Truncated bool
}

type batchRead struct {
	Sources  []batchSource
	Deferred []sourcequeue.Source
}

func New(root string, store Store, queue *sourcequeue.Queue, manager Manager, logger *log.Logger) *Runner {
	return &Runner{Root: root, Store: store, Queue: queue, Manager: manager, Log: logger}
}

func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	if r.Store == nil || r.Queue == nil || r.Manager == nil || strings.TrimSpace(r.Root) == "" {
		return 0, nil
	}
	settings, err := agentsettings.LoadMemorySettings(r.Store)
	if err != nil {
		return 0, err
	}
	if !settings.Enabled {
		return 0, nil
	}
	agent := acp.CanonicalAgentName(settings.Agent)
	if agent == "" || agent == acp.AgentJaz {
		return 0, nil
	}
	reserved, err := r.Queue.Reserve(ctx, r.batchFiles())
	if err != nil {
		return 0, err
	}
	if len(reserved) == 0 {
		return 0, nil
	}
	batch, err := r.readBatch(reserved)
	if err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	if len(batch.Sources) == 0 {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, nil
	}
	defaults, err := agentsettings.LoadAgentDefaults(r.Store)
	if errors.Is(err, storage.ErrSettingNotFound) {
		defaults = agentsettings.DefaultAgentDefaults()
	} else if err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	prompt, err := sourcePrompt(r.Root, batch.Sources)
	if err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	spawned, err := r.Manager.Spawn(ctx, acp.SpawnRequest{
		ACPAgent:        agent,
		Slug:            fmt.Sprintf("memory-source-%s-%s", agent, stamp),
		Title:           "Memory Source Capture",
		Directory:       r.Root,
		Model:           agentsettings.WorkerAgentModel(agent, defaults),
		ReasoningEffort: agentsettings.WorkerAgentReasoningEffort(agent),
		SourceType:      storage.SourceMemorySource,
		SourceID:        stamp,
	})
	if err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	if _, err := r.Manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    prompt,
		Completion: acp.CompletionInline,
	}); err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	job, err := r.Manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: Timeout})
	if err != nil {
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, err
	}
	if job.State == acp.StateRunning || job.State == acp.StateStarting {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = r.Manager.Cancel(cancelCtx, spawned.SessionID)
		_ = r.Queue.Release(context.Background(), reserved)
		return 0, fmt.Errorf("memory source capture timed out after %s", Timeout)
	}
	if job.State != acp.StateIdle {
		_ = r.Queue.Release(context.Background(), reserved)
		if strings.TrimSpace(job.Error) != "" {
			return 0, fmt.Errorf("memory source capture failed: %s", job.Error)
		}
		return 0, fmt.Errorf("memory source capture finished with state %q", job.State)
	}
	if len(batch.Deferred) > 0 {
		if err := r.Queue.Release(ctx, batch.Deferred); err != nil {
			return 0, err
		}
	}
	complete := completedSources(batch.Sources)
	if err := r.Queue.Complete(ctx, complete); err != nil {
		return 0, err
	}
	return len(complete), nil
}

func (r *Runner) readBatch(pending []sourcequeue.Source) (batchRead, error) {
	remaining := r.batchChars()
	out := make([]batchSource, 0, min(len(pending), r.batchFiles()))
	var deferred []sourcequeue.Source
	for i, source := range pending {
		path, err := sourcePath(r.Root, source.Path)
		if err != nil {
			return batchRead{}, err
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			out = append(out, batchSource{Path: source.Path, PendingAt: source.PendingAt})
			continue
		}
		if err != nil {
			return batchRead{}, err
		}
		content := string(data)
		truncated := false
		if len(content) > remaining {
			if len(out) > 0 {
				deferred = append(deferred, pending[i:]...)
				break
			}
			content = content[:remaining]
			truncated = true
		}
		remaining -= len(content)
		out = append(out, batchSource{Path: source.Path, PendingAt: source.PendingAt, Content: content, Truncated: truncated})
		if remaining <= 0 {
			deferred = append(deferred, pending[i+1:]...)
			break
		}
	}
	return batchRead{Sources: out, Deferred: deferred}, nil
}

func completedSources(sources []batchSource) []sourcequeue.Source {
	complete := make([]sourcequeue.Source, 0, len(sources))
	for _, source := range sources {
		complete = append(complete, sourcequeue.Source{Path: source.Path, PendingAt: source.PendingAt})
	}
	return complete
}

func sourcePrompt(root string, sources []batchSource) (string, error) {
	data := memorysourceprompt.Data{
		Root:    root,
		Sources: make([]memorysourceprompt.Source, 0, len(sources)),
	}
	for _, source := range sources {
		data.Sources = append(data.Sources, memorysourceprompt.Source{
			Path:      source.Path,
			Truncated: source.Truncated,
			Content:   source.Content,
		})
	}
	return memorysourceprompt.Render(data)
}

func sourcePath(root, rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source path escapes memory root")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, rel))
	if err != nil {
		return "", err
	}
	check, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(check) || check == ".." || strings.HasPrefix(check, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source path escapes memory root")
	}
	return pathAbs, nil
}

func (r *Runner) batchFiles() int {
	if r.BatchFiles > 0 {
		return r.BatchFiles
	}
	return DefaultBatchFiles
}

func (r *Runner) batchChars() int {
	if r.BatchChars > 0 {
		return r.BatchChars
	}
	return DefaultBatchChars
}
