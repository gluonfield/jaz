package acp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

func (m *Manager) Status(ref string) (Job, error) {
	if job, err := m.job(ref); err == nil {
		return job.Snapshot(), nil
	}
	session, err := m.store.LoadSession(ref)
	if err != nil {
		return Job{}, err
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return Job{}, fmt.Errorf("session %s is not acp-backed", ref)
	}
	job := jobFromSession(session, session.RuntimeRef.Agent, session.RuntimeRef.SessionID, session.RuntimeRef.Cwd, inactiveJobState(session))
	job.Error = session.Error
	return job, nil
}

func (m *Manager) StreamStatus(ref string) (StreamView, error) {
	if job, err := m.job(ref); err == nil {
		return job.streamView(), nil
	}
	job, err := m.Status(ref)
	return StreamViewFromJob(job), err
}

// SessionUsed reports whether the session has had a job since the manager
// started, so a thread whose idle process stopped is still in use.
func (m *Manager) SessionUsed(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, used := m.used[id]
	return used
}

// WithStoppedSessions holds off resumes while worktrees of stopped sessions
// are removed. A live process retains its directory even between turns.
func (m *Manager) WithStoppedSessions(ids []string, remove func() error) error {
	defer m.resumeLocks.Lock(ids...)()
	for _, id := range ids {
		if m.jobByID(id) != nil || m.AwaitsAnswer(id) {
			return nil
		}
	}
	return remove()
}

func (m *Manager) List() []Job {
	m.mu.RLock()
	jobs := make([]*jobState, 0, len(m.jobsByID))
	for _, job := range m.jobsByID {
		jobs = append(jobs, job)
	}
	m.mu.RUnlock()
	out := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, job.Snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (m *Manager) HydrationJobs(ids []string) map[string]HydrationView {
	m.mu.RLock()
	jobs := make(map[string]*jobState, len(ids))
	for _, id := range ids {
		if job := m.jobsByID[id]; job != nil {
			jobs[id] = job
		}
	}
	m.mu.RUnlock()
	views := make(map[string]HydrationView, len(jobs))
	for id, job := range jobs {
		views[id] = job.hydrationView()
	}
	return views
}

// inactiveJobState is the state of a thread without a live job: a running one
// is starting, as when its claimed turn waits for its agent.
func inactiveJobState(session storage.Session) string {
	switch session.Status {
	case storage.StatusError:
		return StateFailed
	case storage.StatusRunning:
		return StateStarting
	}
	return StateNotRunning
}

func (m *Manager) job(ref string) (*jobState, error) {
	ref = strings.TrimSpace(ref)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if job := m.jobsByID[ref]; job != nil {
		return job, nil
	}
	if job := m.jobsBySlug[ref]; job != nil {
		return job, nil
	}
	if job := m.jobsByACP[ref]; job != nil {
		return job, nil
	}
	return nil, fmt.Errorf("active acp session not found: %s", ref)
}

func (m *Manager) jobByACP(acpSessionID string) *jobState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByACP[acpSessionID]
}

// Thinking reports whether sessionID's agent holds its turn's latest prompt
// and has called no tool yet: it is reasoning or writing. A turn whose agent
// is still starting, or that waits to retry a provider failure, is not.
func (m *Manager) Thinking(sessionID string) bool {
	job := m.jobByID(sessionID)
	if job == nil {
		return false
	}
	job.mu.RLock()
	defer job.mu.RUnlock()
	return job.State == StateRunning && job.turn != nil && job.turn.firstPromptSentClosed && len(job.ToolCalls) == 0
}

func (m *Manager) jobByID(id string) *jobState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByID[id]
}
