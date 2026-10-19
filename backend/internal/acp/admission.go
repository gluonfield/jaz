package acp

import "errors"

// ErrClosed refuses new work while Jaz shuts down.
var ErrClosed = errors.New("jaz is restarting; try again once it is back")

// admit holds Close off until the caller releases, or refuses once Close has
// begun: Close stops whatever it finds in place, and nothing new comes after.
func (m *Manager) admit() (func(), error) {
	m.admission.RLock()
	if m.closed {
		m.admission.RUnlock()
		return nil, ErrClosed
	}
	return m.admission.RUnlock, nil
}

func (m *Manager) isClosed() bool {
	release, err := m.admit()
	if err == nil {
		release()
	}
	return err != nil
}

// register makes a started agent's job live unless Jaz shuts down, so Close
// stops every process it did not refuse.
func (m *Manager) register(job *jobState, process *agentProcess) error {
	release, err := m.admit()
	if err != nil {
		return err
	}
	defer release()
	m.addJob(job, process)
	return nil
}
