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
	if execution.Thread != "" {
		// The run waits in its bot thread's queue, after the user's messages;
		// the queue begins it when its turn starts.
		if err := r.server.QueueInternalTurn(context.Background(), execution.Thread, storage.QueuedMessage{Text: loop.Prompt, Output: &storage.TurnOutput{Run: run.ID}}); err != nil {
			r.finishLoopRun(execution, loops.RunStatusError, err.Error())
		}
		return
	}
	startCtx, cancel := serverACPBootstrapContext()
	defer cancel()
	// An empty agent stays empty: the ACP manager resolves the canonical
	// default at spawn time, the same default sessions get.
	agent := strings.TrimSpace(loop.ACPAgent)
	directory := strings.TrimSpace(loop.Directory)
	if directory == "" {
		directory = "."
	}
	spawned, err := r.server.ACP.Spawn(startCtx, acp.SpawnRequest{
		ACPAgent:               agent,
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
	_, prompt, err := execution.Controller.Begin(run.ID, spawned.SessionID)
	if err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
		return
	}
	sendCtx, cancel := serverACPBootstrapContext()
	defer cancel()
	if _, err := r.server.ACP.Send(sendCtx, acp.SendRequest{
		Session:    spawned.SessionID,
		Message:    prompt,
		Completion: acp.CompletionAsync,
	}); err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
	}
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
