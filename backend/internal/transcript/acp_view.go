package transcript

import (
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func sessionSnapshot(session storage.Session, active map[string]acp.HydrationView) acp.Job {
	if session.Status == storage.StatusError {
		return failedSessionSnapshot(session)
	}
	if job, ok := active[session.ID]; ok {
		return canonicalJob(withSessionLabels(session, job.Job()))
	}
	return canonicalJob(inactiveJob(jobFromSession(session)))
}

func childSnapshots(parentID string, events []sessionevents.Event, children []storage.TranscriptSession, active map[string]acp.HydrationView) ([]acp.Job, []sessionevents.ACPPermission) {
	visible := make(map[string]struct{})
	for _, event := range events {
		if event.ACP != nil && event.ACP.ID != "" && event.ACP.ID != parentID {
			visible[event.ACP.ID] = struct{}{}
		}
	}
	out := make([]acp.Job, 0, len(children))
	var permissions []sessionevents.ACPPermission
	for _, child := range children {
		state := jobFromTranscriptSession(child)
		switch {
		case child.Status == storage.StatusError:
			state.State = acp.StateFailed
			state.Error = child.Error
		case active[child.ID].ID != "":
			state = withTranscriptSessionLabels(child, active[child.ID].Job())
		default:
			state = inactiveJob(state)
		}
		_, state.ParentVisible = visible[child.ID]
		state = canonicalJob(state)
		permissions = append(permissions, activePermissions(state.Permissions)...)
		state.Permissions = nil
		state.Plan = nil
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	sort.Slice(permissions, func(i, j int) bool { return permissions[i].ID < permissions[j].ID })
	return out, permissions
}

func jobFromSession(session storage.Session) acp.Job {
	job := acp.Job{
		ID: session.ID, Slug: session.Slug, Title: session.Title, ParentID: session.ParentID,
		ModelProvider: session.ModelProvider, Model: session.Model, ReasoningEffort: session.ReasoningEffort,
		State: acp.StateNotRunning, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}
	if session.RuntimeRef != nil {
		job.ACPAgent = session.RuntimeRef.Agent
		job.ACPSession = session.RuntimeRef.SessionID
		job.Cwd = session.RuntimeRef.Cwd
	}
	return job
}

func jobFromTranscriptSession(session storage.TranscriptSession) acp.Job {
	return acp.Job{
		ID: session.ID, Slug: session.Slug, Title: session.Title, ParentID: session.ParentID,
		ACPAgent: session.ACPAgent, ACPSession: session.ACPSession, Cwd: session.Cwd,
		ModelProvider: session.ModelProvider, Model: session.Model, ReasoningEffort: session.ReasoningEffort,
		State: acp.StateNotRunning, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}
}

func failedSessionSnapshot(session storage.Session) acp.Job {
	job := jobFromSession(session)
	job.State = acp.StateFailed
	job.Error = session.Error
	return canonicalJob(job)
}

func withSessionLabels(session storage.Session, job acp.Job) acp.Job {
	job.ID = first(job.ID, session.ID)
	job.Slug = first(session.Slug, job.Slug)
	job.Title = first(session.Title, job.Title)
	job.ParentID = first(session.ParentID, job.ParentID)
	job.ModelProvider = first(session.ModelProvider, job.ModelProvider)
	job.Model = first(session.Model, job.Model)
	job.ReasoningEffort = first(session.ReasoningEffort, job.ReasoningEffort)
	if job.CreatedAt.IsZero() {
		job.CreatedAt = session.CreatedAt
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = session.UpdatedAt
	}
	return job
}

func withTranscriptSessionLabels(session storage.TranscriptSession, job acp.Job) acp.Job {
	job.ID = first(job.ID, session.ID)
	job.Slug = first(session.Slug, job.Slug)
	job.Title = first(session.Title, job.Title)
	job.ParentID = first(session.ParentID, job.ParentID)
	job.ModelProvider = first(session.ModelProvider, job.ModelProvider)
	job.Model = first(session.Model, job.Model)
	job.ReasoningEffort = first(session.ReasoningEffort, job.ReasoningEffort)
	if job.CreatedAt.IsZero() {
		job.CreatedAt = session.CreatedAt
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = session.UpdatedAt
	}
	return job
}

func inactiveJob(job acp.Job) acp.Job {
	if job.State == acp.StateStarting || job.State == acp.StateRunning || job.State == acp.StateNotRunning {
		job.State = acp.StateIdle
	}
	job.GoalRequested = false
	job.ActiveOperation = ""
	job.Plan = nil
	job.Permissions = nil
	return job
}

func activePermissions(in []sessionevents.ACPPermission) []sessionevents.ACPPermission {
	out := make([]sessionevents.ACPPermission, 0, len(in))
	for _, permission := range in {
		switch strings.ToLower(permission.Status) {
		case "selected", "cancelled", "canceled":
		default:
			if permission.ID != "" {
				out = append(out, permission)
			}
		}
	}
	return out
}

func canonicalJob(job acp.Job) acp.Job {
	if agent := acp.CanonicalAgentName(job.ACPAgent); agent != "" {
		job.ACPAgent = agent
	}
	return job
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
