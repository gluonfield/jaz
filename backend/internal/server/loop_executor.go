package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

type LoopRunner struct {
	native loopNativeHost
	store  storage.Store
	acp    ACPManager
	log    *log.Logger
	agent  bool
}

type loopNativeHost interface {
	nativeSessionDefaults() (storage.CreateSession, error)
	resolveWorkingDir(directory string) (string, error)
	runClaimedNativeSession(context.Context, storage.Session, string) string
	logger() *log.Logger
}

func NewLoopRunner(server *Server) *LoopRunner {
	return &LoopRunner{
		native: server,
		store:  server.Store,
		acp:    server.ACP,
		log:    server.logger().WithPrefix("loops"),
		agent:  server.Agent != nil,
	}
}

func (r *LoopRunner) StartLoopRun(ctx context.Context, execution loops.Execution) {
	loop := execution.Loop
	switch loop.Runtime {
	case "", loops.RuntimeACP:
		r.startACPLoopRun(execution)
	case loops.RuntimeNative:
		r.startNativeLoopRun(ctx, execution)
	default:
		r.finishLoopRun(execution, loops.RunStatusError, fmt.Sprintf("unknown loop runtime %q", loop.Runtime))
	}
}

func (r *LoopRunner) startNativeLoopRun(ctx context.Context, execution loops.Execution) {
	if r.native == nil || !r.agent {
		r.finishLoopRun(execution, loops.RunStatusError, "native agent is not configured")
		return
	}
	loop := execution.Loop
	run := execution.Run
	input, err := r.native.nativeSessionDefaults()
	if err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
		return
	}
	input.Slug = loopRunSlug(loop, run)
	input.Title = loop.Name
	input.SourceType = storage.SourceLoopRun
	input.SourceID = run.ID
	if loop.ReasoningEffort != "" {
		input.ReasoningEffort = loop.ReasoningEffort
	}
	// Per-loop model overrides mirror session creation: switching providers
	// invalidates the settings default model, falling back to the provider's.
	if loop.ModelProvider != "" && loop.ModelProvider != input.ModelProvider {
		meta, _ := provider.NativeProviderByID(loop.ModelProvider)
		input.ModelProvider = loop.ModelProvider
		input.Model = strings.TrimSpace(meta.DefaultModel)
	}
	if loop.Model != "" {
		input.Model = loop.Model
	}
	if directory := strings.TrimSpace(loop.Directory); directory != "" {
		cwd, err := r.native.resolveWorkingDir(directory)
		if err != nil {
			r.finishLoopRun(execution, loops.RunStatusError, err.Error())
			return
		}
		input.RuntimeRef = &storage.RuntimeRef{
			Type:        storage.RuntimeNative,
			Cwd:         cwd,
			ProjectPath: projectPathForRequest(directory, cwd),
		}
	}
	session, err := r.store.CreateSession(input)
	if err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
		return
	}
	if err := execution.Controller.MarkRunning(run.ID, session.ID); err != nil {
		r.logger().Error("mark loop run running failed", "run", run.ID, "error", err)
	}
	status := r.native.runClaimedNativeSession(ctx, session, execution.Prompt)
	if status == storage.StatusIdle {
		r.finishLoopRun(execution, loops.RunStatusOK, "")
		return
	}
	errText := "native loop run failed"
	if loaded, err := r.store.LoadSession(session.ID); err == nil && loaded.Error != "" {
		errText = loaded.Error
	}
	r.finishLoopRun(execution, loops.RunStatusError, errText)
}

func (r *LoopRunner) startACPLoopRun(execution loops.Execution) {
	if r.acp == nil {
		r.finishLoopRun(execution, loops.RunStatusError, "acp manager is not configured")
		return
	}
	loop := execution.Loop
	run := execution.Run
	startCtx, cancel := serverActionContext()
	defer cancel()
	agent := strings.TrimSpace(loop.ACPAgent)
	if agent == "" {
		agent = "codex"
	}
	directory := strings.TrimSpace(loop.Directory)
	if directory == "" {
		directory = "."
	}
	spawned, err := r.acp.Spawn(startCtx, acp.SpawnRequest{
		ACPAgent:        agent,
		Slug:            loopRunSlug(loop, run),
		Title:           loop.Name,
		Directory:       directory,
		Model:           loop.Model,
		ReasoningEffort: loop.ReasoningEffort,
		SourceType:      storage.SourceLoopRun,
		SourceID:        run.ID,
	})
	if err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
		return
	}
	if err := execution.Controller.MarkRunning(run.ID, spawned.SessionID); err != nil {
		r.logger().Error("mark loop run running failed", "run", run.ID, "session", spawned.SessionID, "error", err)
	}
	sendCtx, cancel := serverActionContext()
	defer cancel()
	if _, err := r.acp.Send(sendCtx, acp.SendRequest{
		Session:     spawned.SessionID,
		Message:     execution.Prompt,
		Completion:  acp.CompletionAsync,
		Interactive: true,
	}); err != nil {
		r.finishLoopRun(execution, loops.RunStatusError, err.Error())
	}
}

func (r *LoopRunner) finishLoopRun(execution loops.Execution, status, errText string) {
	if execution.Controller == nil {
		return
	}
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
	if r.log != nil {
		return r.log
	}
	if r.native != nil {
		return r.native.logger()
	}
	return log.Default()
}
