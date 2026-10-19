package acp

import (
	"context"
	"sort"
	"time"
)

// IdleTimeouts says how long an agent process may sit idle before it is
// stopped. The sweep runs every Thread/10, so Thread must not exceed Bot.
type IdleTimeouts struct {
	Thread time.Duration
	// Bot applies to Claude bots, whose relaunch renders their changing prompt
	// and so rewrites their whole prompt cache. It outlasts Claude Code's
	// one-hour subscription cache, which a bot idle this long has lost anyway.
	Bot time.Duration
}

// DefaultIdleTimeouts: resuming takes a few seconds and keeps a thread's
// prompt cache, while an idle process holds hundreds of megabytes.
var DefaultIdleTimeouts = IdleTimeouts{Thread: 10 * time.Minute, Bot: time.Hour}

func (t IdleTimeouts) of(job *jobState, process *agentProcess) time.Duration {
	if rendersPromptOnLaunch(job.ACPAgent, process.mcpPolicy) {
		return t.Bot
	}
	return t.Thread
}

// StopIdleProcesses stops, until ctx ends, the agent process of every thread
// idle for its timeout. The thread's next message resumes its native session,
// as it would after a restart.
func (m *Manager) StopIdleProcesses(ctx context.Context, timeouts IdleTimeouts) {
	ticker := time.NewTicker(timeouts.Thread / 10)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.stopIdle(now, timeouts, memoryPressure())
		}
	}
}

func (m *Manager) stopIdle(now time.Time, timeouts IdleTimeouts, pressure bool) {
	quiet := func(job *jobState, process *agentProcess) bool {
		timeout := timeouts.of(job, process)
		if pressure {
			timeout = min(timeout, time.Minute)
		}
		return process.conn.quietSince(now.Add(-timeout))
	}
	for _, candidate := range m.quietJobs(quiet) {
		job, process := candidate.job, candidate.process
		// Recheck after the slower checks: incoming traffic can change eligibility.
		if !m.subagentsRunning(job) && !m.AwaitsAnswer(job.ID) && quiet(job, process) && job.retire() {
			m.stopRetired(job)
			// Reclaim one early process per sweep, allowing the OS to recover.
			pressure = false
		}
	}
}

type idleProcess struct {
	job     *jobState
	process *agentProcess
	traffic int64
}

func (m *Manager) quietJobs(quiet func(*jobState, *agentProcess) bool) []idleProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var jobs []idleProcess
	for id, process := range m.processes {
		// Only a process Jaz launched has stderr, and only one Jaz launched
		// must restore its sessions; a URL agent has no local process to stop.
		if job := m.jobsByID[id]; process.stderr != nil && quiet(job, process) {
			jobs = append(jobs, idleProcess{job, process, process.conn.lastTraffic.Load()})
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].traffic < jobs[j].traffic })
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
	if j.retired || j.turn != nil || j.finishing != nil || j.leases > 0 || j.backgroundTaskLive() {
		return false
	}
	j.retired = true
	return true
}
