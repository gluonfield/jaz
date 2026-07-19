package sessions

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func mobileEvents(events []sessionevents.Event) []sessionevents.Event {
	out := append([]sessionevents.Event(nil), events...)
	for i := range out {
		if out[i].ACP == nil {
			continue
		}
		state := *out[i].ACP
		state.ToolCalls = mobileToolCalls(state.ToolCalls)
		out[i].ACP = &state
	}
	return out
}

func mobileJobs(jobs []acp.Job) []acp.Job {
	out := append([]acp.Job(nil), jobs...)
	for i := range out {
		out[i] = mobileJob(out[i])
	}
	return out
}

func mobileJob(job acp.Job) acp.Job {
	job.ToolCalls = mobileToolCalls(job.ToolCalls)
	return job
}

func mobileToolCalls(calls []sessionevents.ACPToolCall) []sessionevents.ACPToolCall {
	out := make([]sessionevents.ACPToolCall, len(calls))
	for i, call := range calls {
		out[i] = sessionevents.ACPToolCall{ID: call.ID, Title: call.Title, Status: call.Status}
	}
	return out
}
