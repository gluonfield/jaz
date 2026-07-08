package acp

import (
	"strconv"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const acpTranscriptFlushInterval = 100 * time.Millisecond

type acpTranscriptRun struct {
	eventType         string
	content           string
	upstreamMessageID string
	textRunID         string
}

type acpTranscriptBuffers struct {
	mu        sync.Mutex
	bySession map[string]*acpTranscriptBuffer
}

type acpTranscriptBuffer struct {
	sessionID string
	mu        sync.Mutex
	publishMu sync.Mutex
	timer     *time.Timer
	runs      []acpTranscriptRun

	turnKey        string
	runSeq         int
	currentRunType string
	currentRunID   string
}

func (m *Manager) queueACPMessage(job *jobState, content string) {
	m.queueACPMessageWithID(job, content, "")
}

func (m *Manager) queueACPMessageWithID(job *jobState, content string, upstreamMessageID string) {
	m.queueACPTranscript(job, acpTranscriptRun{eventType: "acp_message", content: content, upstreamMessageID: upstreamMessageID})
}

func (m *Manager) queueACPThought(job *jobState, content string) {
	m.queueACPThoughtWithID(job, content, "")
}

func (m *Manager) queueACPThoughtWithID(job *jobState, content string, upstreamMessageID string) {
	m.queueACPTranscript(job, acpTranscriptRun{eventType: "acp_thought", content: content, upstreamMessageID: upstreamMessageID})
}

func (m *Manager) queueACPTranscript(job *jobState, run acpTranscriptRun) {
	if run.content == "" {
		return
	}
	if job == nil {
		return
	}
	job.mu.RLock()
	sessionID := job.ID
	turnKey := transcriptTurnKey(job.turn)
	job.mu.RUnlock()
	if sessionID == "" {
		return
	}
	buffer := m.transcriptBuffers.get(sessionID, true)
	buffer.queue(m, run, turnKey)
}

func (m *Manager) withACPTranscriptBarrier(job Job, publish func()) {
	if job.ID == "" {
		if publish != nil {
			publish()
		}
		return
	}
	buffer := m.transcriptBuffers.get(job.ID, false)
	if buffer == nil {
		if publish != nil {
			publish()
		}
		return
	}
	buffer.withBarrier(m, job, publish)
}

func (m *Manager) flushACPTranscriptBuffer(buffer *acpTranscriptBuffer) {
	if buffer == nil {
		return
	}
	buffer.flushTimer(m)
}

func (b *acpTranscriptBuffers) get(sessionID string, create bool) *acpTranscriptBuffer {
	if sessionID == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bySession == nil {
		if !create {
			return nil
		}
		b.bySession = map[string]*acpTranscriptBuffer{}
	}
	buffer := b.bySession[sessionID]
	if buffer == nil && create {
		buffer = &acpTranscriptBuffer{sessionID: sessionID}
		b.bySession[sessionID] = buffer
	}
	return buffer
}

func (b *acpTranscriptBuffers) delete(sessionID string) {
	if sessionID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bySession != nil {
		delete(b.bySession, sessionID)
	}
}

func (b *acpTranscriptBuffer) queue(m *Manager, run acpTranscriptRun, turnKey string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run.textRunID = b.textRunIDLocked(run.eventType, run.upstreamMessageID, turnKey)
	if n := len(b.runs); n > 0 && run.textRunID != "" && b.runs[n-1].eventType == run.eventType && b.runs[n-1].textRunID == run.textRunID {
		b.runs[n-1].content += run.content
	} else {
		b.runs = append(b.runs, run)
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(acpTranscriptFlushInterval, func() {
			m.flushACPTranscriptBuffer(b)
		})
	}
}

func textRunID(upstreamMessageID string) string {
	if upstreamMessageID != "" {
		return "message:" + upstreamMessageID
	}
	return ""
}

func transcriptTurnKey(turn *activeTurn) string {
	if turn == nil {
		return ""
	}
	return strconv.FormatInt(turn.startedAt.UnixNano(), 10)
}

func (b *acpTranscriptBuffer) textRunIDLocked(eventType, upstreamMessageID, turnKey string) string {
	if upstreamMessageID != "" {
		b.currentRunType = ""
		b.currentRunID = ""
		return textRunID(upstreamMessageID)
	}
	if turnKey == "" {
		return ""
	}
	if b.turnKey != turnKey {
		b.turnKey = turnKey
		b.runSeq = 0
		b.currentRunType = ""
		b.currentRunID = ""
	}
	if b.currentRunID != "" && b.currentRunType == eventType {
		return b.currentRunID
	}
	b.runSeq++
	b.currentRunType = eventType
	b.currentRunID = "turn:" + turnKey + ":" + strconv.Itoa(b.runSeq)
	return b.currentRunID
}

func (b *acpTranscriptBuffer) withBarrier(m *Manager, job Job, publish func()) {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	runs := b.drainBarrier()
	m.publishACPTranscriptRuns(job, runs)
	if publish != nil {
		publish()
	}
}

func (b *acpTranscriptBuffer) flushTimer(m *Manager) {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	runs := b.drain()
	if len(runs) == 0 {
		return
	}
	var job Job
	if live := m.jobByID(b.sessionID); live != nil {
		job = live.Snapshot()
	}
	m.publishACPTranscriptRuns(job, runs)
}

func (b *acpTranscriptBuffer) drain() []acpTranscriptRun {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.drainLocked()
}

func (b *acpTranscriptBuffer) drainBarrier() []acpTranscriptRun {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentRunType = ""
	b.currentRunID = ""
	return b.drainLocked()
}

func (b *acpTranscriptBuffer) drainLocked() []acpTranscriptRun {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	runs := append([]acpTranscriptRun(nil), b.runs...)
	b.runs = nil
	return runs
}

func (m *Manager) publishACPTranscriptRuns(job Job, runs []acpTranscriptRun) {
	if job.ID == "" || len(runs) == 0 {
		return
	}
	m.saveACPState(job)
	for _, sessionID := range childSessionIDs(&job) {
		events := make([]sessionevents.Event, 0, len(runs))
		envelope := acpTranscriptEnvelope(job)
		for _, run := range runs {
			acp := *envelope
			event := sessionevents.Event{
				SessionID: sessionID,
				Type:      run.eventType,
				ACP:       &acp,
				At:        time.Now().UTC(),
			}
			if run.eventType == "acp_thought" {
				event.ACP.Thought = run.content
			} else {
				event.Content = run.content
			}
			event.ACP.TextRunID = run.textRunID
			events = append(events, event)
		}
		m.recordAndPublishEventsDirect(sessionID, events)
	}
}
