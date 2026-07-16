package acp

import (
	"context"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

type detachedAgentProcess struct {
	conn   jsonrpc.MessageConn
	peer   *jsonrpc.Peer
	cancel context.CancelFunc
}

func claudeProcessPerTurn(name string, cfg AgentConfig) bool {
	return CanonicalAgentName(name) == AgentClaude && cfg.URL == "" && !cfg.Local
}

func (m *Manager) Close() {
	m.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.cancelByID))
	peers := make([]*jsonrpc.Peer, 0, len(m.peersByID))
	conns := make([]jsonrpc.MessageConn, 0, len(m.connsByID))
	jobs := make([]*jobState, 0, len(m.jobsByID))
	for _, cancel := range m.cancelByID {
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	for _, peer := range m.peersByID {
		peers = append(peers, peer)
	}
	for _, conn := range m.connsByID {
		conns = append(conns, conn)
	}
	for _, job := range m.jobsByID {
		jobs = append(jobs, job)
	}
	m.connsByID = map[string]jsonrpc.MessageConn{}
	m.peersByID = map[string]*jsonrpc.Peer{}
	m.cancelByID = map[string]context.CancelFunc{}
	m.mu.Unlock()

	stopping := make([]bool, len(jobs))
	activeTurns := 0
	for i, job := range jobs {
		stopping[i] = job.requestShutdown()
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
	m.log.Info("acp manager closing", "jobs", len(jobs), "active_turns", activeTurns, "peers", len(peers), "conns", len(conns), "cancels", len(cancels))
	for _, cancel := range cancels {
		cancel()
	}
	for _, peer := range peers {
		_ = peer.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	for i, job := range jobs {
		if stopping[i] {
			job.setState(StateCancelled, StopReasonServerShutdown, "")
		}
		m.withACPTranscriptBarrier(job.Snapshot(), nil)
		m.transcriptBuffers.delete(job.ID)
	}
}

func (m *Manager) detachProcessAfterTurn(job *jobState) *detachedAgentProcess {
	if !job.processPerTurn {
		return nil
	}
	m.mu.Lock()
	process := &detachedAgentProcess{
		conn:   m.connsByID[job.ID],
		peer:   m.peersByID[job.ID],
		cancel: m.cancelByID[job.ID],
	}
	delete(m.connsByID, job.ID)
	delete(m.peersByID, job.ID)
	delete(m.cancelByID, job.ID)
	m.mu.Unlock()
	return process
}

func (m *Manager) closeDetachedProcess(job *jobState, process *detachedAgentProcess) {
	if process == nil {
		return
	}
	m.withACPTranscriptBarrier(job.Snapshot(), nil)
	m.transcriptBuffers.delete(job.ID)
	closeAgentProcess(process.peer, process.conn, process.cancel)
}

func (m *Manager) teardown(id string) {
	if job := m.jobByID(id); job != nil {
		m.withACPTranscriptBarrier(job.Snapshot(), nil)
	}
	m.transcriptBuffers.delete(id)
	m.mu.Lock()
	job := m.jobsByID[id]
	conn := m.connsByID[id]
	peer := m.peersByID[id]
	cancel := m.cancelByID[id]
	delete(m.jobsByID, id)
	delete(m.connsByID, id)
	delete(m.peersByID, id)
	delete(m.cancelByID, id)
	delete(m.serveErrByID, id)
	if job != nil {
		delete(m.jobsBySlug, job.Slug)
		delete(m.jobsByACP, job.ACPSession)
	}
	m.mu.Unlock()
	closeAgentProcess(peer, conn, cancel)
}

func closeAgentProcess(peer *jsonrpc.Peer, conn jsonrpc.MessageConn, cancel context.CancelFunc) {
	if peer != nil {
		_ = peer.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if cancel != nil {
		cancel()
	}
}
