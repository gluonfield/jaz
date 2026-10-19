package acp

import "github.com/wins/jaz/backend/internal/storage"

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
