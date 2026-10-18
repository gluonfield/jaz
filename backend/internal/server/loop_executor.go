package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

type LoopRunner struct {
	server *Server
}

func NewLoopRunner(server *Server) *LoopRunner {
	return &LoopRunner{server: server}
}

func (r *LoopRunner) StartLoopRun(_ context.Context, execution loops.Execution) {
	loop := execution.Loop
	switch loop.Runtime {
	case "", loops.RuntimeACP:
		r.startACPLoopRun(execution)
	default:
		r.finishLoopRun(execution, loops.RunStatusError, fmt.Sprintf("unknown loop runtime %q", loop.Runtime))
	}
}

func (r *LoopRunner) startACPLoopRun(execution loops.Execution) {
	if r.server.ACP == nil {
		r.finishLoopRun(execution, loops.RunStatusError, "acp manager is not configured")
		return
	}
	loop := execution.Loop
	run := execution.Run
	mode := execution.Thread
	if mode == loops.ThreadMain && r.contextFull(loop.BotID) {
		mode = loops.ThreadChild
	}
	thread := loop.BotID
	output := &storage.TurnOutput{Run: run.ID}
	if mode != loops.ThreadMain {
		startCtx, cancel := serverACPBootstrapContext()
		defer cancel()
		// An empty agent stays empty: the ACP manager resolves the canonical
		// default at spawn time, the same default sessions get. A bot's child
		// starts in the bot's home unless the routine names a directory.
		directory := strings.TrimSpace(loop.Directory)
		if directory == "" && mode != loops.ThreadChild {
			directory = "."
		}
		spawned, err := r.server.ACP.Spawn(startCtx, acp.SpawnRequest{
			ACPAgent:               strings.TrimSpace(loop.ACPAgent),
			Slug:                   loopRunSlug(loop, run),
			Title:                  loop.Name,
			ParentID:               loop.BotID,
			Directory:              directory,
			ModelProvider:          loop.ModelProvider,
			Model:                  loop.Model,
			ReasoningEffort:        loop.ReasoningEffort,
			SourceType:             storage.SourceLoopRun,
			SourceID:               run.ID,
			ArtifactSurface:        execution.ArtifactSurface,
			SystemPromptExtensions: execution.SystemPromptExtensions,
		})
		if err != nil {
			r.finishLoopRun(execution, loops.RunStatusError, err.Error())
			return
		}
		thread = spawned.SessionID
		// A bot's child speaks as the bot does: what it sends reaches the bot,
		// and a run with nothing to say leaves the bot alone.
		if mode == loops.ThreadChild {
			output.ReplyTo = loop.BotID
		}
	}
	// The run waits in its thread's queue, after the user's messages; the
	// queue begins it when its turn starts.
	if err := r.server.QueueInternalTurn(context.Background(), thread, storage.QueuedMessage{Text: loop.Prompt, Output: output}); err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
	}
}

// contextFull reports whether a bot's thread is at least 80% full by its
// provider's own count, so a run meant for it does better in a child. A fill
// the provider does not report keeps the run in the thread.
func (r *LoopRunner) contextFull(botID string) bool {
	session, err := r.server.Store.LoadSession(botID)
	usage := session.Usage
	return err == nil && usage.ContextWindowTokens > 0 && usage.ContextTokens*5 >= usage.ContextWindowTokens*4
}

// LoopRunStatus is the status a finished turn gives the loop run it served.
func LoopRunStatus(state string) string {
	switch state {
	case acp.StateFailed:
		return loops.RunStatusError
	case acp.StateCancelled:
		return loops.RunStatusCancelled
	}
	return loops.RunStatusOK
}

func (r *LoopRunner) finishLoopRun(execution loops.Execution, status, errText string) {
	if err := execution.Controller.Finish(execution.Run.ID, status, errText); err != nil {
		r.logger().Error("finish loop run failed", "run", execution.Run.ID, "status", status, "error", err)
	}
}

func loopRunSlug(loop loops.Loop, run loops.Run) string {
	when := run.ScheduledFor
	if when.IsZero() {
		when = time.Now().UTC()
	}
	return fmt.Sprintf("loop-%s-%s", loop.Name, when.Local().Format("20060102-1504"))
}

func (r *LoopRunner) logger() *log.Logger {
	return r.server.logger().WithPrefix("loops")
}
