package acp

import "github.com/wins/jaz/backend/internal/sessionevents"

type ThreadSnapshot struct {
	ThreadID      string                        `json:"threadId"`
	Slug          string                        `json:"slug"`
	Title         string                        `json:"title,omitempty"`
	Agent         string                        `json:"agent"`
	State         string                        `json:"state"`
	Directory     string                        `json:"directory,omitempty"`
	Model         string                        `json:"model,omitempty"`
	ModelProvider string                        `json:"modelProvider,omitempty"`
	Thinking      string                        `json:"thinking,omitempty"`
	Assistant     string                        `json:"assistant,omitempty"`
	Truncated     bool                          `json:"truncated,omitempty"`
	Plan          []sessionevents.PlanEntry     `json:"plan,omitempty"`
	Permissions   []sessionevents.ACPPermission `json:"permissions,omitempty"`
	Error         string                        `json:"error,omitempty"`
	Budget        *BudgetDecision               `json:"budget,omitempty"`
}

func snapshotThread(job Job) ThreadSnapshot {
	text := []rune(job.Assistant)
	truncated := len(text) > 4000
	if truncated {
		text = text[len(text)-4000:]
	}
	return ThreadSnapshot{
		ThreadID: job.ID, Slug: job.Slug, Title: job.Title, Agent: job.ACPAgent,
		State: job.State, Directory: job.Cwd, Model: job.Model, ModelProvider: job.ModelProvider, Thinking: job.ReasoningEffort,
		Assistant: string(text), Truncated: truncated, Plan: job.Plan, Permissions: job.Permissions, Error: job.Error,
	}
}
