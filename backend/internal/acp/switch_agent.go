package acp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// SwitchAgent moves an idle ACP thread to another agent with that agent's
// default model and effort. The old agent's process is retired and the next
// turn starts a new native session, which keeps no memory of the earlier
// conversation while the Jaz transcript stays. An agent_switch event records
// the move, so resuming knows a fresh native session is intended.
func (m *Manager) SwitchAgent(_ context.Context, sessionID, agent string) error {
	defer m.resumeLocks.Lock(sessionID)()
	if job, err := m.job(sessionID); err == nil && job.turnDone() != nil {
		return errors.New("wait for the active turn before switching agent")
	}
	session, err := m.store.LoadSession(sessionID)
	if err != nil {
		return err
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return fmt.Errorf("session %s is not acp-backed", sessionID)
	}
	req, cfg, effort, err := m.spawnConfig(SpawnRequest{
		ACPAgent:        agent,
		SourceType:      session.SourceType,
		ArtifactSurface: session.RuntimeRef.ArtifactSurface,
		MCPServerPolicy: session.RuntimeRef.MCPServerPolicy,
	})
	if err != nil {
		return err
	}
	if req.ACPAgent == CanonicalAgentName(session.RuntimeRef.Agent) {
		return nil
	}
	m.teardown(sessionID)
	session.RuntimeRef.Agent = req.ACPAgent
	session.RuntimeRef.SessionID = ""
	session.ModelProvider = sessionModelProvider(req.ACPAgent, cfg)
	session.Model = strings.TrimSpace(cfg.Model)
	session.ReasoningEffort = effort
	if err := m.store.SaveSession(session); err != nil {
		return err
	}
	events := []sessionevents.Event{{SessionID: sessionID, Type: sessionevents.TypeAgentSwitch, Content: req.ACPAgent, At: time.Now().UTC()}}
	if err := m.store.AppendSessionEvents(sessionID, events...); err != nil {
		return err
	}
	if m.Events != nil {
		m.Events.Publish(events[0])
	}
	m.publishSessionChanged(sessionID)
	m.log.Info("switched agent", "session", sessionID, "agent", req.ACPAgent)
	return nil
}

// SetModel picks the model and reasoning effort an ACP thread runs with and
// stores them on the thread. Before the current agent's first turn the stored
// choice is applied when its native session starts, as a new thread's is.
// After that it goes through the agent's own model and effort options first.
func (m *Manager) SetModel(ctx context.Context, sessionID, model, effort string) error {
	unlock := m.resumeLocks.Lock(sessionID)
	session, cfg, effort, live, err := m.modelChoice(sessionID, model, effort)
	if err == nil && !live {
		err = m.saveModel(session, cfg, effort)
	}
	unlock()
	if err != nil || !live {
		return err
	}
	if err := m.setLiveModel(ctx, sessionID, cfg, effort); err != nil {
		return err
	}
	defer m.resumeLocks.Lock(sessionID)()
	if session, err = m.store.LoadSession(sessionID); err != nil {
		return err
	}
	return m.saveModel(session, cfg, effort)
}

// modelChoice validates a pick for a thread's current agent and reports
// whether the agent already has a native session or a process to apply it to.
func (m *Manager) modelChoice(sessionID, model, effort string) (storage.Session, AgentConfig, string, bool, error) {
	session, err := m.store.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, AgentConfig{}, "", false, err
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return storage.Session{}, AgentConfig{}, "", false, fmt.Errorf("session %s is not acp-backed", sessionID)
	}
	_, cfg, effort, err := m.spawnConfig(SpawnRequest{ACPAgent: session.RuntimeRef.Agent, Model: model, ReasoningEffort: effort})
	job, _ := m.job(sessionID)
	return session, cfg, effort, session.RuntimeRef.SessionID != "" || job != nil, err
}

func (m *Manager) saveModel(session storage.Session, cfg AgentConfig, effort string) error {
	session.ModelProvider = sessionModelProvider(CanonicalAgentName(session.RuntimeRef.Agent), cfg)
	session.Model = strings.TrimSpace(cfg.Model)
	session.ReasoningEffort = effort
	if err := m.store.SaveSession(session); err != nil {
		return err
	}
	m.publishSessionChanged(session.ID)
	return nil
}

func (m *Manager) setLiveModel(ctx context.Context, sessionID string, cfg AgentConfig, effort string) error {
	job, err := m.resume(ctx, sessionID)
	if err != nil {
		return err
	}
	if job, err = m.acquireSessionProcess(ctx, job); err != nil {
		return err
	}
	defer job.release()
	policy := agentPolicyForAgent(job.ACPAgent)
	if err := m.setConfigCategory(ctx, job, "model", policy.sessionConfigModel(cfg)); err != nil || policy.effortEncodedInModel(cfg.Model) {
		return err
	}
	return m.setConfigCategory(ctx, job, "thought_level", policy.sessionConfigEffort(effort))
}

// setConfigCategory sets the agent's live option of one category, such as its
// model, which the agent advertises under an id of its own.
func (m *Manager) setConfigCategory(ctx context.Context, job *jobState, category, value string) error {
	if value == "" {
		return nil
	}
	job.mu.RLock()
	i := slices.IndexFunc(job.agentSession.ConfigOptions, func(option sessionevents.AgentConfigOption) bool { return option.Category == category })
	var option sessionevents.AgentConfigOption
	if i >= 0 {
		option = job.agentSession.ConfigOptions[i]
	}
	job.mu.RUnlock()
	if i < 0 {
		return fmt.Errorf("agent %q offers no %s option", CanonicalAgentName(job.ACPAgent), category)
	}
	if option.CurrentValue == value {
		return nil
	}
	return m.setSessionConfig(ctx, job, option.ID, value)
}
