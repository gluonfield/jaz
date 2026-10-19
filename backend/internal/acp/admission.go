package acp

import (
	"context"
	"errors"
)

func (m *Manager) beginLaunch(ctx context.Context) (context.Context, func(), error) {
	release, err := m.admit()
	if err != nil {
		return nil, nil, err
	}
	m.launches.Add(1)
	release()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.launchCtx, cancel)
	return ctx, func() {
		stop()
		cancel()
		m.launches.Done()
	}, nil
}

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
	m.admission.RLock()
	defer m.admission.RUnlock()
	return m.closed
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
