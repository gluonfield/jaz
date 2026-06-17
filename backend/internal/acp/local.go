package acp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

type LocalAgentRunner interface {
	Run(context.Context, LocalAgentRequest) <-chan agent.StreamEvent
}

type LocalAgentRequest struct {
	Session       storage.Session
	Message       string
	Attachments   []storage.Attachment
	PlanRequested bool
}

func (m *Manager) RegisterLocalAgent(name string, runner LocalAgentRunner) {
	name = CanonicalAgentName(name)
	if name == "" || runner == nil {
		return
	}
	m.mu.Lock()
	m.localAgents[name] = runner
	m.mu.Unlock()
}

func (m *Manager) localAgent(name string) LocalAgentRunner {
	name = CanonicalAgentName(name)
	m.mu.RLock()
	runner := m.localAgents[name]
	m.mu.RUnlock()
	return runner
}

func (m *Manager) configuredLocal(name string) bool {
	cfg, ok, err := m.configuredAgent(name)
	return err == nil && ok && cfg.Local
}

func (m *Manager) newLocalJob(session storage.Session, agentName, cwd string) *Job {
	return &Job{
		ID:         session.ID,
		Slug:       session.Slug,
		Title:      session.Title,
		ParentID:   session.ParentID,
		ACPAgent:   CanonicalAgentName(agentName),
		ACPSession: session.ID,
		Cwd:        cwd,
		State:      StateIdle,
		Modes:      localModeState(),
		CreatedAt:  session.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
		toolByID:   make(map[string]ToolCallSnapshot),
	}
}

func localModeState() ModeState {
	return ModeState{
		CurrentModeID: "default",
		PlanModeID:    "plan",
		AvailableModes: []ModeSnapshot{
			{ID: "default", Name: "Default"},
			{ID: "plan", Name: "Plan"},
		},
	}
}

func (m *Manager) spawnLocalSession(session storage.Session, agentName, cwd string) (SpawnResult, error) {
	session.RuntimeRef.SessionID = session.ID
	if err := m.store.SaveSession(session); err != nil {
		session.Status = storage.StatusError
		session.Error = err.Error()
		_ = m.store.SaveSession(session)
		return SpawnResult{}, err
	}
	job := m.newLocalJob(session, agentName, cwd)
	m.addJob(job, nil, nil, nil)
	m.saveACPState(job.Snapshot())
	m.log.Info("spawned local agent session", "agent", job.ACPAgent, "session", job.ID)
	return SpawnResult{
		Status:    "created",
		SessionID: job.ID,
		Slug:      job.Slug,
		ACPAgent:  job.ACPAgent,
		Cwd:       job.Cwd,
		State:     StateIdle,
		Session:   session,
	}, nil
}

func (m *Manager) resumeLocalSession(session storage.Session, agentName string, cfg AgentConfig) (*Job, error) {
	var state storage.ACPState
	if loader, ok := m.store.(acpStateLoader); ok {
		state, _ = loader.LoadACPState(session.ID)
	}
	cwd := firstNonEmpty(session.RuntimeRef.Cwd, state.Cwd)
	if cwd == "" {
		var err error
		if cwd, err = m.resolveCwd(cfg.Cwd); err != nil {
			return nil, err
		}
	}
	changed := false
	if session.RuntimeRef.SessionID == "" {
		session.RuntimeRef.SessionID = session.ID
		changed = true
	}
	if session.ModelProvider == "" {
		session.ModelProvider = strings.TrimSpace(cfg.ModelProvider)
		changed = true
	}
	if session.Model == "" {
		session.Model = strings.TrimSpace(cfg.Model)
		changed = true
	}
	if session.ReasoningEffort == "" {
		session.ReasoningEffort = strings.TrimSpace(cfg.ReasoningEffort)
		changed = true
	}
	if changed {
		_ = m.store.SaveSession(session)
	}
	job := m.newLocalJob(session, agentName, cwd)
	job.ParentVisible = state.ParentVisible
	m.addJob(job, nil, nil, nil)
	m.saveACPState(job.Snapshot())
	m.log.Info("resumed local agent session", "agent", job.ACPAgent, "session", job.ID)
	return job, nil
}

func (m *Manager) runLocalPrompt(ctx context.Context, job *Job, runner LocalAgentRunner, message string, attachments []storage.Attachment) {
	job.turnMu.Lock()
	defer job.turnMu.Unlock()

	job.mu.RLock()
	done := job.done
	planRequested := job.planRequested
	job.mu.RUnlock()
	if done == nil {
		done = job.startTurn(CompletionInline, false, false, false)
	}
	session, err := m.store.LoadSession(job.ID)
	if err != nil {
		m.failTurn(job, err)
		m.finishTurn(done, job)
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.setCancelFunc(job.ID, cancel)
	defer m.clearCancelFunc(job.ID, cancel)
	defer cancel()

	finalState := StateFailed
	finalError := "Agent stream ended without a completion event."
	failed := false
	for event := range runner.Run(runCtx, LocalAgentRequest{
		Session:       session,
		Message:       message,
		Attachments:   attachments,
		PlanRequested: planRequested,
	}) {
		switch event.Type {
		case agent.StreamDelta:
			m.applyLocalMessage(job, event.Delta)
		case agent.StreamReasoning:
			m.applyLocalThought(job, event.Reasoning)
		case agent.StreamToolCall:
			if event.ToolCall != nil {
				m.applyLocalToolCall(job, *event.ToolCall)
			}
		case agent.StreamToolResult:
			m.applyLocalToolResult(job, event.ToolName, event.Error)
		case agent.StreamDone:
			if event.Usage != nil {
				m.recordUsage(job, storage.Usage{
					InputTokens:           event.Usage.InputTokens,
					CachedInputTokens:     event.Usage.CachedInputTokens,
					CachedWriteTokens:     event.Usage.CachedWriteTokens,
					OutputTokens:          event.Usage.OutputTokens,
					ReasoningOutputTokens: event.Usage.ReasoningOutputTokens,
					TotalTokens:           event.Usage.TotalTokens,
				})
			}
			if !failed {
				finalState = StateIdle
				finalError = ""
			}
		case agent.StreamError:
			failed = true
			finalState = StateFailed
			finalError = firstNonEmpty(event.Error, "local agent turn failed")
		}
	}
	if runCtx.Err() != nil && finalState == StateFailed {
		finalState = StateCancelled
		finalError = ""
	}
	if jobCancelRequested(job) {
		finalState = StateCancelled
		finalError = ""
	}
	if finalState == StateIdle {
		job.setState(StateIdle, "", "")
	} else if finalState == StateCancelled {
		job.setState(StateCancelled, "cancelled", "")
	} else {
		job.setState(StateFailed, "", finalError)
	}
	m.publishACPStatus(job.Snapshot())
	m.finishTurn(done, job)
}

func (m *Manager) applyLocalMessage(job *Job, chunk string) {
	if chunk == "" {
		return
	}
	job.mu.Lock()
	job.Assistant = appendACPText(job.Assistant, chunk)
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	snapshot := job.Snapshot()
	m.publishACPMessage(snapshot, chunk)
}

func (m *Manager) applyLocalThought(job *Job, chunk string) {
	if chunk == "" {
		return
	}
	job.mu.Lock()
	job.Thought = appendACPText(job.Thought, chunk)
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	snapshot := job.Snapshot()
	m.publishACPThought(snapshot, chunk)
}

func (m *Manager) applyLocalToolCall(job *Job, call provider.ToolCall) {
	id := provider.ToolCallID(call)
	title := provider.ToolCallName(call)
	if id == "" {
		id = fmt.Sprintf("tool-%d", time.Now().UnixNano())
	}
	if title == "" {
		title = id
	}
	snapshot, tool := m.updateLocalTool(job, ToolCallSnapshot{ID: id, Title: title, Status: "running"})
	_ = m.store.UpsertActivity(job.ID, storage.ActivityEntry{
		ID:     tool.ID,
		Kind:   "tool",
		Text:   firstNonEmpty(tool.Title, tool.ID),
		Status: tool.Status,
		At:     time.Now().UTC(),
	})
	m.publishACPTool(snapshot, tool)
}

func (m *Manager) applyLocalToolResult(job *Job, title, errText string) {
	status := "completed"
	if strings.TrimSpace(errText) != "" {
		status = "failed"
	}
	job.mu.RLock()
	var id string
	for i := len(job.ToolCalls) - 1; i >= 0; i-- {
		call := job.ToolCalls[i]
		if terminalToolStatus(call.Status) {
			continue
		}
		if title == "" || call.Title == title || call.ID == title {
			id = call.ID
			break
		}
	}
	job.mu.RUnlock()
	if id == "" {
		return
	}
	snapshot, tool := m.updateLocalTool(job, ToolCallSnapshot{ID: id, Status: status})
	_ = m.store.UpsertActivity(job.ID, storage.ActivityEntry{
		ID:     tool.ID,
		Kind:   "tool",
		Text:   firstNonEmpty(tool.Title, tool.ID),
		Status: tool.Status,
		At:     time.Now().UTC(),
	})
	m.publishACPTool(snapshot, tool)
}

func (m *Manager) updateLocalTool(job *Job, next ToolCallSnapshot) (Job, ToolCallSnapshot) {
	job.mu.Lock()
	current := job.toolByID[next.ID]
	current.ID = next.ID
	if next.Title != "" {
		current.Title = next.Title
	}
	if next.Status != "" {
		current.Status = next.Status
	}
	job.toolByID[current.ID] = current
	job.ToolCalls = sortedToolCalls(job.toolByID)
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	snapshot := job.Snapshot()
	return snapshot, current
}

func (m *Manager) setCancelFunc(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	m.cancelByID[id] = cancel
	m.mu.Unlock()
}

func (m *Manager) clearCancelFunc(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	delete(m.cancelByID, id)
	m.mu.Unlock()
}

func (m *Manager) cancelFunc(id string) context.CancelFunc {
	m.mu.RLock()
	cancel := m.cancelByID[id]
	m.mu.RUnlock()
	return cancel
}
