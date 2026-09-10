package acp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (m *Manager) applyBackgroundTask(job *jobState, raw json.RawMessage) bool {
	var update struct {
		Kind             string  `json:"sessionUpdate"`
		ID               string  `json:"asyncTaskId"`
		Name             string  `json:"name"`
		Type             string  `json:"taskType"`
		State            string  `json:"state"`
		Description      *string `json:"description"`
		Summary          *string `json:"summary"`
		LastToolName     *string `json:"lastToolName"`
		OutputFilePath   *string `json:"outputFilePath"`
		ToolCallID       *string `json:"toolCallId"`
		CanStop          bool    `json:"canStop"`
		ShowInTranscript bool    `json:"showInTranscript"`
		Usage            *struct {
			TotalTokens int64 `json:"totalTokens"`
			ToolUses    int64 `json:"toolUses"`
			DurationMs  int64 `json:"durationMs"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &update) != nil || update.ID == "" {
		return false
	}
	switch update.Kind {
	case "async_task_spawned", "async_task_progress", "async_task_state_update":
	default:
		return false
	}
	job.mu.Lock()
	if job.backgroundTasks == nil {
		job.backgroundTasks = make(map[string]sessionevents.AgentTask)
	}
	task := job.backgroundTasks[update.ID]
	task.ID = update.ID
	task.ConnectionLost = false
	if update.Kind == "async_task_spawned" {
		task.Name = update.Name
		task.Type = update.Type
		task.CanStop = update.CanStop
		task.ShowInTranscript = update.ShowInTranscript
		if task.State == "" {
			task.State = "running"
		}
	}
	if update.State != "" {
		task.State = update.State
	}
	for _, field := range []struct {
		dst *string
		src *string
	}{
		{&task.Description, update.Description}, {&task.Summary, update.Summary},
		{&task.LastToolName, update.LastToolName}, {&task.OutputFilePath, update.OutputFilePath},
		{&task.ToolCallID, update.ToolCallID},
	} {
		if field.src != nil {
			*field.dst = *field.src
		}
	}
	if update.Usage != nil {
		task.Usage = &sessionevents.AgentTaskUsage{TotalTokens: update.Usage.TotalTokens, ToolUses: update.Usage.ToolUses, DurationMs: update.Usage.DurationMs}
	}
	if task.State != "running" && task.State != "paused" {
		task.CanStop = false
	}
	job.backgroundTasks[task.ID] = task
	job.mu.Unlock()
	m.publishBackgroundTask(job, task)
	return true
}

func (m *Manager) publishBackgroundTask(job *jobState, task sessionevents.AgentTask) {
	m.publishOrderedACPEvents(job.eventView(), sessionevents.Event{
		SessionID: job.ID, Type: sessionevents.TypeAgentTask, AgentTask: &task,
		ProjectionKey: "agent_task:" + task.ID, ProjectionOp: sessionevents.ProjectionReplace,
	})
}

func (m *Manager) disconnectBackgroundTasks(job *jobState) {
	job.mu.Lock()
	var changed []sessionevents.AgentTask
	for id, task := range job.backgroundTasks {
		if (task.State == "running" || task.State == "paused") && !task.ConnectionLost {
			task.ConnectionLost = true
			job.backgroundTasks[id] = task
			changed = append(changed, task)
		}
	}
	job.mu.Unlock()
	for _, task := range changed {
		m.publishBackgroundTask(job, task)
	}
}

func (m *Manager) StopBackgroundTask(ctx context.Context, session, taskID string) error {
	job, err := m.job(session)
	if err != nil {
		return err
	}
	job.mu.RLock()
	task, exists := job.backgroundTasks[taskID]
	job.mu.RUnlock()
	if !exists || !task.CanStop || task.ConnectionLost {
		return fmt.Errorf("background task %q is not stoppable", taskID)
	}
	peer := m.peer(job.ID)
	if peer == nil {
		return fmt.Errorf("agent connection is unavailable")
	}
	raw, err := peer.Call(ctx, "_session/async_task/stop", map[string]string{
		"sessionId": job.ACPSession, "asyncTaskId": taskID,
	})
	if err != nil {
		return err
	}
	var response struct {
		Stopped bool `json:"stopped"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return err
	}
	if !response.Stopped {
		return fmt.Errorf("agent could not stop background task %q", taskID)
	}
	return nil
}
