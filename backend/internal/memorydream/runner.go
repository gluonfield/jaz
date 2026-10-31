package memorydream

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/memorydreamprompt"
)

const Timeout = 45 * time.Minute

type Manager interface {
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Wait(context.Context, acp.WaitRequest) (acp.Job, error)
	Cancel(context.Context, string) (acp.Job, error)
}

type Runner struct {
	Store   storage.SettingsStorage
	Manager Manager
	Queue   *sourcequeue.Queue
}

func New(store storage.SettingsStorage, manager Manager, queue *sourcequeue.Queue) *Runner {
	return &Runner{Store: store, Manager: manager, Queue: queue}
}

func (r *Runner) RunDream(ctx context.Context, req jazmem.DreamRequest) (report jazmem.DreamReport, err error) {
	settings, err := agentsettings.LoadMemorySettings(r.Store)
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	agent := acp.CanonicalAgentName(settings.Agent)
	if agent == "" {
		return jazmem.DreamReport{}, fmt.Errorf("memory agent is not configured")
	}
	if agent == acp.AgentJaz {
		return jazmem.DreamReport{}, fmt.Errorf("built-in Jaz cannot be used as the memory agent yet")
	}
	agentDefaults, err := agentsettings.LoadAgentDefaults(r.Store)
	if errors.Is(err, storage.ErrSettingNotFound) {
		agentDefaults = agentsettings.DefaultAgentDefaults()
	} else if err != nil {
		return jazmem.DreamReport{}, err
	}
	sources, err := r.Queue.Reserve(ctx, math.MaxInt)
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	defer func() {
		if len(sources) > 0 {
			err = errors.Join(err, r.Queue.Release(context.WithoutCancel(ctx), sources))
		}
	}()
	date := req.Date
	if date.IsZero() {
		date = time.Now()
	}
	date = date.Local()
	suffix := fmt.Sprintf("%s-%d", date.Format("2006-01-02-1504"), time.Now().UnixNano())
	runSlug := "dreams/runs/" + suffix
	reviewSlug := "dreams/review/dream-" + suffix
	receipt := filepath.Join(req.Root, ".state", "consolidation", suffix+".json")
	if err := os.MkdirAll(filepath.Dir(receipt), 0o755); err != nil {
		return jazmem.DreamReport{}, err
	}
	defer os.Remove(receipt)
	manifest := filepath.Join(filepath.Dir(receipt), suffix+".sources.json")
	if err := writeSources(manifest, sources); err != nil {
		return jazmem.DreamReport{}, err
	}
	defer os.Remove(manifest)
	prompt, err := agentPrompt(req, runSlug, reviewSlug, receipt, manifest)
	if err != nil {
		return jazmem.DreamReport{}, err
	}

	spawned, err := r.Manager.Spawn(ctx, acp.SpawnRequest{
		ACPAgent:        agent,
		Slug:            fmt.Sprintf("memory-dream-%s-%s", agent, suffix),
		Title:           "Memory Dream " + date.Format("2006-01-02 15:04"),
		Directory:       req.Root,
		Model:           settings.WorkerModel(agentDefaults),
		ReasoningEffort: settings.WorkerReasoningEffort(agentDefaults),
		SourceType:      storage.SourceMemoryDream,
		SourceID:        suffix,
	})
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	active := true
	defer func() {
		if active {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, cancelErr := r.Manager.Cancel(cancelCtx, spawned.SessionID)
			err = errors.Join(err, cancelErr)
		}
	}()
	if _, err := r.Manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    prompt,
		Completion: acp.CompletionInline,
	}); err != nil {
		return jazmem.DreamReport{}, err
	}
	job, err := r.Manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: Timeout})
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	if job.State == acp.StateRunning || job.State == acp.StateStarting {
		return jazmem.DreamReport{}, fmt.Errorf("memory dream timed out after %s", Timeout)
	}
	active = false
	if job.State != acp.StateIdle {
		if strings.TrimSpace(job.Error) != "" {
			return jazmem.DreamReport{}, fmt.Errorf("memory dream failed: %s", job.Error)
		}
		return jazmem.DreamReport{}, fmt.Errorf("memory dream finished with state %q", job.State)
	}
	runPage, err := os.ReadFile(filepath.Join(req.Root, filepath.FromSlash(runSlug)+".md"))
	if err != nil {
		return jazmem.DreamReport{}, fmt.Errorf("read consolidation report: %w", err)
	}
	if strings.TrimSpace(string(runPage)) == "" {
		return jazmem.DreamReport{}, errors.New("consolidation report is empty")
	}
	completed, pending, err := readReceipt(receipt, sources)
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	if err := r.Queue.Settle(ctx, completed, pending); err != nil {
		return jazmem.DreamReport{}, err
	}
	sources = nil
	var warnings []string
	if len(pending) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d source pages remain for the next consolidation", len(pending)))
	}
	return jazmem.DreamReport{
		RunSlug:    runSlug,
		ReviewSlug: reviewSlug,
		ModelUsed:  "acp:" + agent,
		Warnings:   warnings,
	}, nil
}

func agentPrompt(req jazmem.DreamRequest, runSlug, reviewSlug, receipt, manifest string) (string, error) {
	return memorydreamprompt.Render(memorydreamprompt.Data{
		Root:            req.Root,
		RunSlug:         runSlug,
		ReviewSlug:      reviewSlug,
		ReceiptPath:     receipt,
		SourcesPath:     manifest,
		LongTermPolicy:  jazmem.LongTermDreamGuidance(),
		ShortTermPolicy: jazmem.ShortTermDreamGuidance(),
	})
}
