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
	return jobFromSession(session, session.RuntimeRef.Agent, session.RuntimeRef.SessionID, session.RuntimeRef.Cwd, inactiveJobState(session)), nil
}

func (m *Manager) StreamStatus(ref string) (StreamView, error) {
	if job, err := m.job(ref); err == nil {
		return job.streamView(), nil
	}
	job, err := m.Status(ref)
	return StreamViewFromJob(job), err
}

func (m *Manager) LiveSessionRefs() []string {
	m.mu.RLock()
	refs := make([]string, 0, len(m.jobsByID)+len(m.jobsBySlug))
	for id := range m.jobsByID {
		refs = append(refs, id)
	}
	for slug := range m.jobsBySlug {
		refs = append(refs, slug)
	}
	m.mu.RUnlock()
	return refs
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

func inactiveJobState(session storage.Session) string {
	if session.Status == storage.StatusError {
		return StateFailed
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

func (m *Manager) jobByID(id string) *jobState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobsByID[id]
}
