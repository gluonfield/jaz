package acp

import (
	"context"
	"sync"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

type agentProcess struct {
	conn       jsonrpc.MessageConn
	peer       *jsonrpc.Peer
	cancel     context.CancelFunc
	stderr     *processStderrTail
	turnScoped bool
	leases     int
	serveErr   error
	serveDone  chan struct{}
}

func newAgentProcess(ac *agentConn, turnScoped bool) *agentProcess {
	return &agentProcess{
		conn:       ac.conn,
		peer:       ac.peer,
		cancel:     ac.cancel,
		stderr:     ac.stderr,
		turnScoped: turnScoped,
		serveDone:  make(chan struct{}),
	}
}

func turnScopedAgentProcess(cfg AgentConfig) bool {
	return cfg.URL == "" && !cfg.Local
}

func (p *agentProcess) close() {
	if p.peer != nil {
		_ = p.peer.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	if p.cancel != nil {
		p.cancel()
	}
}

type processLease struct {
	once    sync.Once
	manager *Manager
	job     *jobState
	process *agentProcess
}

func (l *processLease) Release() {
	if l != nil {
		l.once.Do(func() { l.manager.releaseProcess(l.job, l.process) })
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	processes := make([]*agentProcess, 0, len(m.processes))
	for _, process := range m.processes {
		processes = append(processes, process)
	}
	jobs := make([]*jobState, 0, len(m.jobsByID))
	for _, job := range m.jobsByID {
		jobs = append(jobs, job)
	}
	m.processes = map[string]*agentProcess{}
	m.mu.Unlock()

	stopping := make([]bool, len(jobs))
	activeTurns := 0
	for i, job := range jobs {
		stopping[i] = job.requestShutdown()
		if cancel := job.turnCancel(); cancel != nil {
			cancel()
		}
		if stopping[i] {
			activeTurns++
			snapshot := job.Snapshot()
			m.log.Info(
				"acp manager shutdown stopping turn",
				"session", snapshot.ID,
				"agent", snapshot.ACPAgent,
				"state", snapshot.State,
				"operation", snapshot.ActiveOperation,
			)
		}
	}
	m.log.Info("acp manager closing", "jobs", len(jobs), "active_turns", activeTurns, "processes", len(processes))
	for _, process := range processes {
		process.close()
	}
	for i, job := range jobs {
		if stopping[i] {
			job.setState(StateCancelled, StopReasonServerShutdown, "")
		}
		m.withACPTranscriptBarrier(job.eventSnapshot(), nil)
		m.transcriptBuffers.delete(job.ID)
	}
}

func (m *Manager) acquireSessionProcess(ctx context.Context, job *jobState) (*jobState, *processLease, error) {
	for {
		m.mu.Lock()
		current := m.jobsByID[job.ID]
		process := m.processes[job.ID]
		if current == job && process != nil && process.serveErr == nil {
			if !process.turnScoped {
				m.mu.Unlock()
				return job, nil, nil
			}
			process.leases++
			m.mu.Unlock()
			return job, &processLease{manager: m, job: job, process: process}, nil
		}
		var serveErr error
		inUse := false
		if current == job && process != nil {
			serveErr = process.serveErr
			inUse = process.leases > 0
		}
		m.mu.Unlock()

		if current != nil && current != job {
			job = current
			continue
		}
		if serveErr != nil && (job.turnDone() != nil || inUse) {
			return nil, nil, serveErr
		}
		var err error
		job, err = m.restart(ctx, job)
		if err != nil {
			return nil, nil, err
		}
	}
}

func (m *Manager) releaseProcess(job *jobState, process *agentProcess) {
	m.mu.Lock()
	if m.jobsByID[job.ID] != job || m.processes[job.ID] != process {
		m.mu.Unlock()
		return
	}
	process.leases--
	if !process.turnScoped || process.leases > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.processes, job.ID)
	m.mu.Unlock()
	m.closeProcess(job, process)
}

func (m *Manager) closeUnusedProcess(job *jobState) {
	m.mu.Lock()
	process := m.processes[job.ID]
	if m.jobsByID[job.ID] != job || process == nil || !process.turnScoped || process.leases > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.processes, job.ID)
	m.mu.Unlock()
	m.closeProcess(job, process)
}

func (m *Manager) closeProcess(job *jobState, process *agentProcess) {
	m.withACPTranscriptBarrier(job.eventSnapshot(), nil)
	m.transcriptBuffers.delete(job.ID)
	process.close()
}

func (m *Manager) teardown(id string) {
	if job := m.jobByID(id); job != nil {
		m.withACPTranscriptBarrier(job.eventSnapshot(), nil)
	}
	m.transcriptBuffers.delete(id)
	m.mu.Lock()
	job := m.jobsByID[id]
	process := m.processes[id]
	delete(m.jobsByID, id)
	delete(m.processes, id)
	if job != nil {
		delete(m.jobsBySlug, job.Slug)
		delete(m.jobsByACP, job.ACPSession)
	}
	m.mu.Unlock()
	if process != nil {
		process.close()
	}
}

func (m *Manager) addJob(job *jobState, process *agentProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsByID[job.ID] = job
	m.jobsBySlug[job.Slug] = job
	if job.ACPSession != "" {
		m.jobsByACP[job.ACPSession] = job
	}
	if process != nil {
		m.processes[job.ID] = process
	}
}

func (m *Manager) peer(id string) *jsonrpc.Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if process := m.processes[id]; process != nil {
		return process.peer
	}
	return nil
}

func (m *Manager) setServeErr(peer *jsonrpc.Peer, err error) {
	id, process := m.processByPeer(peer)
	if process == nil {
		return
	}
	m.recordServeErr(id, process, withProcessStderr(err, process.stderr))
}

func (m *Manager) recordServeErr(id string, process *agentProcess, err error) error {
	var job *jobState
	m.mu.Lock()
	if m.processes[id] != process {
		m.mu.Unlock()
		return err
	}
	if process.serveErr != nil {
		err = process.serveErr
		m.mu.Unlock()
		return err
	}
	process.serveErr = err
	close(process.serveDone)
	job = m.jobsByID[id]
	m.mu.Unlock()
	m.log.Error("acp agent connection failed", "session", id, "error", err)
	if job == nil {
		return err
	}
	job.mu.RLock()
	running := job.State == StateRunning || job.State == StateStarting
	job.mu.RUnlock()
	if running {
		return err
	}
	job.setState(StateFailed, "", err.Error())
	m.publishACPStatus(job.eventSnapshot())
	return err
}

func (m *Manager) processByPeer(peer *jsonrpc.Peer) (string, *agentProcess) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for candidateID, candidate := range m.processes {
		if candidate.peer == peer {
			return candidateID, candidate
		}
	}
	return "", nil
}

func (m *Manager) waitServeErr(peer *jsonrpc.Peer, fallback error) error {
	id, process := m.processByPeer(peer)
	if process == nil {
		return fallback
	}
	select {
	case <-process.serveDone:
	case <-time.After(100 * time.Millisecond):
		return m.recordServeErr(id, process, withProcessStderr(fallback, process.stderr))
	}
	m.mu.RLock()
	err := process.serveErr
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	return fallback
}

func (m *Manager) serveErr(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if process := m.processes[id]; process != nil {
		return process.serveErr
	}
	return nil
}
