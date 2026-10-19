package acp

import (
	"context"
	"time"
)

// IdleProcessTimeout is how long a thread's agent process may sit idle before
// it is stopped. Resuming takes a few seconds and keeps the provider's prompt
// cache; an idle process holds hundreds of megabytes.
const IdleProcessTimeout = 10 * time.Minute

// StopIdleProcesses stops, until ctx ends, the agent process of every thread
// idle for idleFor. The thread's next message resumes its native session, as
// it would after a restart.
func (m *Manager) StopIdleProcesses(ctx context.Context, idleFor time.Duration) {
	ticker := time.NewTicker(idleFor / 10)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			since := now.Add(-idleFor)
			for job, conn := range m.quietJobs(since) {
				// The connection must still be quiet once the slower checks pass:
				// anything the agent reports meanwhile crosses it.
				if !m.subagentsRunning(job) && !m.AwaitsAnswer(job.ID) && conn.quietSince(since) && job.retire() {
					m.stopRetired(job)
				}
			}
		}
	}
}

func (m *Manager) quietJobs(since time.Time) map[*jobState]*promptTrackingConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	jobs := make(map[*jobState]*promptTrackingConn)
	for id, process := range m.processes {
		// Only a process Jaz launched has stderr, and only one Jaz launched
		// must restore its sessions; a URL agent has no local process to stop.
		if process.stderr != nil && process.conn.quietSince(since) {
			jobs[m.jobsByID[id]] = process.conn
		}
	}
	return jobs
}

func (m *Manager) subagentsRunning(job *jobState) bool {
	open, err := m.openProviderSubagents(job.ID)
	if err != nil {
		m.log.Error("load provider subagents", "session", job.ID, "error", err)
		return true
	}
	return len(open) > 0
}

// stopRetired stops a retired job's process. It holds the session's resume
// lock until the process has exited, so a resume never opens the session in
// a second one.
func (m *Manager) stopRetired(job *jobState) {
	defer m.resumeLocks.Lock(job.ID)()
	if m.jobByID(job.ID) != job {
		return
	}
	m.teardown(job.ID)
	m.log.Info("stopped idle agent process", "session", job.ID, "agent", job.ACPAgent)
}

// lease holds the job's process from being stopped as idle until release; it
// fails once the process is being stopped.
func (j *jobState) lease() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.retired {
		return false
	}
	j.leases++
	return true
}

func (j *jobState) release() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.leases--
}

// retire claims the job's process for stopping when no turn, lease or
// background task holds it; none can start once it has.
func (j *jobState) retire() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.retired || j.turn != nil || j.finishing != nil || j.leases > 0 {
		return false
	}
	for _, task := range j.backgroundTasks {
		if taskLive(task) {
			return false
		}
	}
	j.retired = true
	return true
}
