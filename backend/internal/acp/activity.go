package acp

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (j *jobState) recordActivityLocked(phase sessionevents.ACPActivityPhase, reason sessionevents.ACPActivityReason, at time.Time) {
	j.Activity.Phase, j.Activity.Reason, j.Activity.At = phase, reason, at
	if j.turn != nil {
		j.Activity.TurnStartedAt = j.turn.startedAt
	}
}

// Bot chat omits private thought/message chunks. Refresh their safe activity
// snapshot on transitions and at most once per three seconds of streamed text.
func (m *Manager) publishActivityProgress(job *jobState) {
	job.mu.Lock()
	now := time.Now().UTC()
	activity, sent := job.Activity, job.activityPublished
	changed := activity.Phase != sent.Phase || activity.Reason != sent.Reason || !activity.TurnStartedAt.Equal(sent.TurnStartedAt)
	if activity.Phase == "" || activity.At.Equal(sent.At) {
		job.mu.Unlock()
		return
	}
	if delay := 3*time.Second - now.Sub(job.activityPublishedAt); !changed && delay > 0 {
		if job.activityPublishTimer == nil {
			job.activityPublishTimer = time.AfterFunc(delay, func() {
				job.mu.Lock()
				job.activityPublishTimer = nil
				job.mu.Unlock()
				m.publishActivityProgress(job)
			})
		}
		job.mu.Unlock()
		return
	}
	job.activityPublished, job.activityPublishedAt = activity, now
	job.mu.Unlock()
	for _, id := range surfaceSessionIDs(job.eventView()) {
		m.publishSessionChanged(id)
	}
}

func (j *jobState) recordStateActivityLocked(state string, now time.Time) {
	switch state {
	case StateStarting:
		j.recordActivityLocked("starting", "", now)
	case StateIdle, StateCancelled:
		j.recordActivityLocked("idle", "", now)
	case StateFailed:
		j.recordActivityLocked("error", "provider_error", now)
	case StateInterrupted:
		j.recordActivityLocked("waiting", "interrupted", now)
	}
}

func (j *jobState) recordToolActivityLocked(status string, now time.Time) {
	switch status {
	case "pending", "in_progress", "running":
		j.recordActivityLocked("tool", "", now)
	case "completed", "failed", "cancelled":
		j.recordActivityLocked("waiting", "tool_finished", now)
	}
}
