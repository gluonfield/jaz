package memorydream

import (
	"context"
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
	agent := acp.CanonicalAgentName(settings.DreamAgent)
	if agent == "" {
		return jazmem.DreamReport{}, jazmem.ErrDreamRunnerUnavailable
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
		ACPAgent:   agent,
		Slug:       fmt.Sprintf("memory-dream-%s-%s-%d", agent, suffix, time.Now().UnixNano()),
		Title:      "Memory Dream " + runLabel(date),
		Directory:  req.Root,
		SourceType: storage.SourceMemoryDream,
		SourceID:   suffix,
	})
	if err != nil {
		return jazmem.DreamReport{}, err
	}
	if _, err := r.Manager.Send(ctx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    agentPrompt(req, runSlug, reviewSlug),
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

func agentPrompt(req jazmem.DreamRequest, runSlug, reviewSlug string) string {
	return strings.TrimSpace(fmt.Sprintf(`# Memory Dream

You are running Jaz's periodic memory consolidation job as a coding agent.

Work directly in this markdown memory root:
%s

Use at most 100 meaningful tool/edit steps. Read before editing. Do not run jazmem index or other memory maintenance commands; Jaz will reindex after you finish.

Required work:
- Read LONG_TERM.md, SHORT_TERM.md, today's and recent daily pages, recent inbox/source pages, and relevant canonical people/company/project/concept pages.
- If SHORT_TERM.md is oversized or stale, move durable material into canonical pages with citations, then rewrite SHORT_TERM.md so it contains only current focus, active projects, and open loops.
- You are allowed to update LONG_TERM.md when a fact is durable enough for that horizon. Preserve the required heading.
- Capture compressed insights, people/company/network facts, decisions, preferences, relationships, who said what, who is working on what, and happiness/blocker/alignment signals.
- Create canonical pages when the user discussed an entity beyond public-knowledge facts.
- Every durable fact must have an absolute-date source citation.
- Leave uncertain candidates in %s.
- Write a run report to %s summarizing inputs, changed files, promotions, skipped material, and warnings.

Return only a concise final status after edits are complete.`, req.Root, reviewSlug, runSlug))
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
