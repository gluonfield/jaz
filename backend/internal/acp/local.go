package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type LocalAgentRunner interface {
	Run(context.Context, LocalAgentRequest) <-chan agent.StreamEvent
}

type LocalUtilityRunner interface {
	RunUtility(context.Context, LocalUtilityRequest) <-chan agent.StreamEvent
}

type LocalAgentRequest struct {
	Session                storage.Session
	Message                string
	Attachments            []storage.Attachment
	PlanRequested          bool
	ArtifactSurface        string
	SystemPromptExtensions promptmodule.Modules
}

type LocalUtilityRequest struct {
	Session storage.Session
	Message string
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

func (m *Manager) runLocalUtilityPrompt(ctx context.Context, req SpawnRequest, cfg AgentConfig, cwd, message string) (string, error) {
	runner := m.localAgent(req.ACPAgent)
	if runner == nil {
		return "", fmt.Errorf("local acp agent %q is not registered", req.ACPAgent)
	}
	utility, ok := runner.(LocalUtilityRunner)
	if !ok {
		return "", fmt.Errorf("local acp agent %q does not support utility prompts", req.ACPAgent)
	}
	session := storage.Session{
		ID:              "utility",
		Slug:            "utility",
		Runtime:         storage.RuntimeACP,
		ModelProvider:   strings.TrimSpace(cfg.ModelProvider),
		Model:           strings.TrimSpace(cfg.Model),
		ReasoningEffort: strings.TrimSpace(cfg.ReasoningEffort),
		RuntimeRef: &storage.RuntimeRef{
			Type:        storage.RuntimeACP,
			Agent:       req.ACPAgent,
			Cwd:         cwd,
			ProjectPath: cwd,
		},
	}
	var text strings.Builder
	var streamErr error
	events := utility.RunUtility(ctx, LocalUtilityRequest{Session: session, Message: message})
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case event, ok := <-events:
			if !ok {
				if streamErr != nil {
					return "", streamErr
				}
				if out := strings.TrimSpace(text.String()); out != "" {
					return out, nil
				}
				return "", fmt.Errorf("empty utility prompt response")
			}
			switch event.Type {
			case agent.StreamDelta:
				text.WriteString(event.Delta)
			case agent.StreamError:
				if streamErr == nil {
					streamErr = fmt.Errorf("local utility prompt failed: %s", firstNonEmpty(event.Error, "unknown error"))
				}
			}
		}
	}
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

func (m *Manager) spawnLocalSession(session storage.Session, agentName, cwd string, systemPromptExtensions promptmodule.Modules) (SpawnResult, error) {
	session.RuntimeRef.SessionID = session.ID
	if err := m.store.SaveSession(session); err != nil {
		session.Status = storage.StatusError
		session.Error = err.Error()
		_ = m.store.SaveSession(session)
		return SpawnResult{}, err
	}
	job := m.newLocalJob(session, agentName, cwd)
	job.systemPromptExtensions = systemPromptExtensions.Strings()
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

	done, planRequested := job.turnDoneAndPlan()
	if done == nil {
		done = job.startTurn(CompletionInline, false, false)
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
	artifactSurface := ""
	if session.RuntimeRef != nil {
		artifactSurface = session.RuntimeRef.ArtifactSurface
	}
	job.mu.RLock()
	systemPromptExtensions := job.systemPromptExtensions.Strings()
	job.mu.RUnlock()
	for event := range runner.Run(runCtx, LocalAgentRequest{
		Session:                session,
		Message:                message,
		Attachments:            attachments,
		PlanRequested:          planRequested,
		ArtifactSurface:        artifactSurface,
		SystemPromptExtensions: systemPromptExtensions,
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
			m.applyLocalToolResult(job, event.ToolName, event.Result, event.Error)
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
	bufferMessage := job.turn != nil && job.turn.planRequested
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	if bufferMessage {
		return
	}
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
	name := provider.ToolCallName(call)
	title := name
	if id == "" {
		id = fmt.Sprintf("tool-%d", time.Now().UnixNano())
	}
	if title == "" {
		title = id
	}
	next := ToolCallSnapshot{
		ID:       id,
		Title:    title,
		Status:   "running",
		ToolName: name,
		RawInput: boundedRawInput(json.RawMessage(provider.ToolCallArguments(call))),
	}
	snapshot, tool := m.updateLocalTool(job, next)
	_ = m.store.UpsertActivity(job.ID, storage.ActivityEntry{
		ID:     tool.ID,
		Kind:   "tool",
		Text:   firstNonEmpty(tool.Title, tool.ID),
		Status: tool.Status,
		At:     time.Now().UTC(),
	})
	m.publishACPTool(snapshot, tool)
}

func (m *Manager) applyLocalToolResult(job *Job, title, result, errText string) {
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
	snapshot, tool := m.updateLocalTool(job, ToolCallSnapshot{ID: id, Status: status, Content: localToolContent(result, errText)})
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
	mergeToolCall(&current, next)
	job.toolByID[current.ID] = current
	job.ToolCalls = sortedToolCalls(job.toolByID)
	job.UpdatedAt = time.Now().UTC()
	job.mu.Unlock()
	snapshot := job.Snapshot()
	return snapshot, current
}

// localToolContent surfaces a native-agent tool result (or its error) as a
// single normalized text block so native Jaz tool calls render through the same
// path as external ACP agents.
func localToolContent(result, errText string) []sessionevents.ACPToolContent {
	text := strings.TrimSpace(result)
	if text == "" {
		text = strings.TrimSpace(errText)
	}
	if text == "" {
		return nil
	}
	return []sessionevents.ACPToolContent{{Type: "text", Text: clampToolText(text)}}
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
