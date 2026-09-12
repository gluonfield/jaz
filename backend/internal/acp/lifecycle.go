package acp

import (
	"context"
	"time"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/storage"
)

type agentProcess struct {
	conn      jsonrpc.MessageConn
	peer      *jsonrpc.Peer
	cancel    context.CancelFunc
	closed    <-chan struct{}
	stderr    *processStderrTail
	serveErr  error
	serveDone chan struct{}
}

func newAgentProcess(ac *agentConn) *agentProcess {
	return &agentProcess{
		conn:      ac.conn,
		peer:      ac.peer,
		cancel:    ac.cancel,
		closed:    ac.closed,
		stderr:    ac.stderr,
		serveDone: make(chan struct{}),
	}
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
	m.turnReaders = map[string]int{}
	m.pendingDiscard = map[string]*jobState{}
	m.mu.Unlock()

	stopping := make([]bool, len(jobs))
	turns := make([]<-chan struct{}, len(jobs))
	activeTurns := 0
	for i, job := range jobs {
		stopping[i], turns[i] = job.requestShutdown()
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
		if turns[i] != nil {
			<-turns[i]
		}
		if stopping[i] && job.Snapshot().StopReason == StopReasonServerShutdown {
			if err := m.store.UpdateSessionStatus(job.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
				m.log.Error("mark shutdown turn interrupted", "session", job.ID, "error", err)
			}
		}
		m.withACPTranscriptBarrier(job.eventView(), nil)
		m.disconnectBackgroundTasks(job)
		m.transcriptBuffers.delete(job.ID)
	}
}

func (m *Manager) acquireSessionProcess(ctx context.Context, job *jobState) (*jobState, error) {
	for {
		m.mu.RLock()
		current := m.jobsByID[job.ID]
		process := m.processes[job.ID]
		var serveErr error
		if process != nil {
			serveErr = process.serveErr
		}
		m.mu.RUnlock()
		if current != nil && current != job {
			job = current
			continue
		}
		if current == job && process != nil && serveErr == nil {
			return job, nil
		}
		if serveErr != nil && job.turnDone() != nil {
			return nil, serveErr
		}
		var err error
		job, err = m.restart(ctx, job)
		if err != nil {
			return nil, err
		}
	}
}

func (m *Manager) teardown(id string) {
	if job := m.jobByID(id); job != nil {
		m.disconnectBackgroundTasks(job)
		m.withACPTranscriptBarrier(job.eventView(), nil)
	}
	m.transcriptBuffers.delete(id)
	m.projectionMu.Lock()
	delete(m.projections, id)
	m.projectionMu.Unlock()
	m.mu.Lock()
	job := m.jobsByID[id]
	process := m.processes[id]
	delete(m.jobsByID, id)
	delete(m.processes, id)
	delete(m.pendingDiscard, id)
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
	m.disconnectBackgroundTasks(job)
	job.mu.RLock()
	running := job.State == StateRunning || job.State == StateStarting
	job.mu.RUnlock()
	if running {
		return err
	}
	job.setState(StateFailed, "", err.Error())
	m.publishACPStatus(job.eventView())
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
