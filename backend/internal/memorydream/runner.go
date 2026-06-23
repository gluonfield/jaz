package memorydream

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
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
	Log     *log.Logger
}

func New(store storage.SettingsStorage, manager Manager, logger *log.Logger) *Runner {
	return &Runner{Store: store, Manager: manager, Log: logger}
}

func (r *Runner) RunDream(ctx context.Context, req jazmem.DreamRequest) (jazmem.DreamReport, error) {
	settings, err := agentsettings.LoadMemorySettings(r.Store)
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	agent := acp.CanonicalAgentName(settings.Agent)
	if agent == "" {
		return jazmem.DreamReport{}, jazmem.ErrDreamRunnerUnavailable
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
	date := req.Date
	if date.IsZero() {
		date = time.Now()
	}
	date = date.Local()
	suffix := runSuffix(date)
	runSlug := "dreams/runs/" + suffix
	reviewSlug := "dreams/review/dream-" + suffix

	spawned, err := r.Manager.Spawn(ctx, acp.SpawnRequest{
		ACPAgent:        agent,
		Slug:            fmt.Sprintf("memory-dream-%s-%s-%d", agent, suffix, time.Now().UnixNano()),
		Title:           "Memory Dream " + runLabel(date),
		Directory:       req.Root,
		Model:           agentsettings.WorkerAgentModel(agent, agentDefaults),
		ReasoningEffort: agentsettings.WorkerAgentReasoningEffort(agent),
		SourceType:      storage.SourceMemoryDream,
		SourceID:        suffix,
	})
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	prompt, err := agentPrompt(req, runSlug, reviewSlug)
	if err != nil {
		return jazmem.DreamReport{}, err
	}
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
		cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = r.Manager.Cancel(cancelCtx, spawned.SessionID)
		return jazmem.DreamReport{}, fmt.Errorf("memory dream timed out after %s", Timeout)
	}
	if job.State != acp.StateIdle {
		if strings.TrimSpace(job.Error) != "" {
			return jazmem.DreamReport{}, fmt.Errorf("memory dream failed: %s", job.Error)
		}
		return jazmem.DreamReport{}, fmt.Errorf("memory dream finished with state %q", job.State)
	}
	warnings := ensureRunPage(req.Root, runSlug, date, agent, spawned.SessionID, job.Assistant)
	if len(warnings) > 0 && r.Log != nil {
		r.Log.Warn("dream runner wrote fallback run page", "run", runSlug)
	}
	return jazmem.DreamReport{
		RunSlug:   runSlug,
		ModelUsed: "acp:" + agent,
		Warnings:  warnings,
	}, nil
}

func agentPrompt(req jazmem.DreamRequest, runSlug, reviewSlug string) (string, error) {
	return memorydreamprompt.Render(memorydreamprompt.Data{
		Root:            req.Root,
		RunSlug:         runSlug,
		ReviewSlug:      reviewSlug,
		LongTermPolicy:  jazmem.LongTermDreamGuidance(),
		ShortTermPolicy: jazmem.ShortTermDreamGuidance(),
	})
}

func ensureRunPage(root, runSlug string, date time.Time, agent, sessionID, assistant string) []string {
	path := filepath.Join(root, filepath.FromSlash(runSlug)+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return []string{"agent did not write dream run page and fallback creation failed: " + err.Error()}
	}
	assistant = strings.TrimSpace(assistant)
	if len(assistant) > 4000 {
		assistant = assistant[:4000] + "\n...[truncated]"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntitle: \"Dream %s\"\ntype: dream_run\ndate: %s\n---\n\n", runLabel(date), date.Format("2006-01-02"))
	fmt.Fprintf(&b, "# Dream %s\n\n", runLabel(date))
	fmt.Fprintf(&b, "- Model: `acp:%s`\n", agent)
	fmt.Fprintf(&b, "- ACP session: `%s`\n", sessionID)
	b.WriteString("- Warning: agent did not write the requested run page; Jaz created this fallback.\n")
	if assistant != "" {
		b.WriteString("\n## Agent Final Status\n\n")
		b.WriteString(assistant)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return []string{"agent did not write dream run page and fallback creation failed: " + err.Error()}
	}
	return []string{"agent did not write dream run page; Jaz created a fallback run page"}
}

func runSuffix(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02-1504")
}

func runLabel(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
}
