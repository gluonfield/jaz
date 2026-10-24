package acp

import "github.com/wins/jaz/backend/internal/storage"

func (m *Manager) promptSending(job *jobState, id string) error {
	job.mu.Lock()
	defer job.mu.Unlock()
	if err := m.setPromptStateLocked(job, storage.PromptSending); err != nil {
		return err
	}
	if turn := job.turn; turn != nil {
		turn.unsent--
		turn.prompts[id] = struct{}{}
	}
	return nil
}

func (m *Manager) promptAnswered(job *jobState, id string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if turn := job.turn; turn != nil {
		if _, sent := turn.prompts[id]; sent {
			if err := m.setPromptStateLocked(job, storage.PromptConfirmed); err != nil {
				m.log.Error("record prompt confirmation", "session", job.ID, "error", err)
			}
			delete(turn.prompts, id)
			m.dropTakenSteersLocked(job)
		}
	}
}

func (m *Manager) promptSent(job *jobState) {
	job.mu.Lock()
	defer job.mu.Unlock()
	if turn := job.turn; turn != nil {
		turn.closeFirstPromptSent()
		m.dropTakenSteersLocked(job)
	}
}

// dropTakenSteersLocked drops the kept steers the agent took in. An agent
// that queues prompts works on the oldest one sent and has yet to take in
// the rest. It answers a prompt as it takes in the next, or just before,
// when the prompt ends on its own.
func (m *Manager) dropTakenSteersLocked(job *jobState) {
	turn := job.turn
	untaken := turn.unsent + max(len(turn.prompts)-1, 0)
	if turn.kept <= untaken {
		return
	}
	if err := m.store.KeepTurnSteers(job.ID, untaken); err != nil {
		m.log.Error("drop steers taken in", "session", job.ID, "error", err)
		return
	}
	turn.kept = untaken
}

func (m *Manager) setPromptStateLocked(job *jobState, state storage.PromptState) error {
	turn := job.turn
	if turn == nil || turn.promptObserved || state == storage.PromptConfirmed && !turn.promptSending {
		return nil
	}
	if state == storage.PromptConfirmed {
		turn.promptObserved = true
	}
	err := m.store.SetTurnPromptState(job.ID, job.ACPSession, turn.startedAt, state)
	if err == nil && state == storage.PromptSending {
		turn.promptSending = true
	}
	return err
}
