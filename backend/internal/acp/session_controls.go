package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (m *Manager) applySessionControls(job *jobState, raw json.RawMessage) bool {
	var update struct {
		Kind          string          `json:"sessionUpdate"`
		ConfigOptions json.RawMessage `json:"configOptions"`
		Commands      []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Input       *struct {
				Hint string `json:"hint"`
			} `json:"input"`
		} `json:"availableCommands"`
	}
	if json.Unmarshal(raw, &update) != nil {
		return false
	}
	job.mu.Lock()
	switch {
	case update.ConfigOptions != nil:
		model, effort := job.Model, job.ReasoningEffort
		options := translateConfigOptions(update.ConfigOptions)
		for _, previous := range job.agentSession.ConfigOptions {
			switch previous.Category {
			case "model":
				job.Model = ""
			case "thought_level":
				job.ReasoningEffort = ""
			}
		}
		for i := range options {
			option := &options[i]
			// Plan temporarily replaces the permission mode, retaining the user's baseline.
			temporaryPlan := option.Category == "mode" && option.ID != job.Modes.planConfigID && option.CurrentValue == job.Modes.PlanModeID
			for _, previous := range job.agentSession.ConfigOptions {
				if previous.ID == option.ID && previous.UserValue != nil && (option.CurrentValue == *previous.UserValue || temporaryPlan) {
					option.UserValue = previous.UserValue
				}
			}
		}
		job.agentSession.ConfigOptions = options
		for _, option := range job.agentSession.ConfigOptions {
			switch option.Category {
			case "model":
				job.Model = option.CurrentValue
			case "thought_level":
				job.ReasoningEffort = option.CurrentValue
			case "mode":
				if job.Modes.userModeID != "" && option.ID != job.Modes.planConfigID && option.CurrentValue != job.Modes.PlanModeID {
					job.Modes.userModeID = option.CurrentValue
				}
			}
		}
		if job.Model != model || job.ReasoningEffort != effort {
			if err := m.store.UpdateSessionModel(job.ID, job.Model, job.ReasoningEffort); err != nil {
				m.log.Error("persist agent model selection", "session", job.ID, "error", err)
			}
		}
	case update.Kind == "available_commands_update":
		commands := make([]sessionevents.AgentCommand, 0, len(update.Commands))
		for _, command := range update.Commands {
			item := sessionevents.AgentCommand{Name: command.Name, Description: command.Description}
			if command.Input != nil {
				item.InputHint = command.Input.Hint
			}
			commands = append(commands, item)
		}
		job.agentSession.Commands = commands
	default:
		job.mu.Unlock()
		return false
	}
	job.mu.Unlock()
	m.publishAgentSession(job)
	return true
}

func translateConfigOptions(raw json.RawMessage) []sessionevents.AgentConfigOption {
	var options []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Category     string `json:"category"`
		Type         string `json:"type"`
		CurrentValue string `json:"currentValue"`
		Options      []struct {
			Value       string                           `json:"value"`
			Name        string                           `json:"name"`
			Description string                           `json:"description"`
			Options     []sessionevents.AgentConfigValue `json:"options"`
		} `json:"options"`
		Meta configOptionMeta `json:"_meta"`
	}
	if json.Unmarshal(raw, &options) != nil {
		return nil
	}
	result := make([]sessionevents.AgentConfigOption, 0, len(options))
	for _, option := range options {
		if option.Type != "select" {
			continue
		}
		item := sessionevents.AgentConfigOption{
			ID: option.ID, Name: option.Name, Description: option.Description,
			Category: option.Category, CurrentValue: option.CurrentValue,
			RecommendedValue: option.Meta.Jetbrains.Air.RecommendedValue,
			Options:          []sessionevents.AgentConfigValue{},
		}
		for _, value := range option.Options {
			if value.Options != nil {
				for _, child := range value.Options {
					child.Group = value.Name
					item.Options = append(item.Options, child)
				}
			} else {
				item.Options = append(item.Options, sessionevents.AgentConfigValue{
					Value: value.Value, Name: value.Name, Description: value.Description,
				})
			}
		}
		result = append(result, item)
	}
	return result
}

func (m *Manager) publishAgentSession(job *jobState) {
	job.mu.RLock()
	state := job.agentSession
	job.mu.RUnlock()
	m.publishOrderedACPEvents(job.eventView(), sessionevents.Event{
		SessionID: job.ID, Type: sessionevents.TypeAgentSession, AgentSession: &state,
		ProjectionKey: "agent_session", ProjectionOp: sessionevents.ProjectionReplace,
	})
}

func (m *Manager) SetSessionConfig(ctx context.Context, session, id, value string) error {
	job, err := m.resume(ctx, session)
	if err != nil {
		return err
	}
	job, err = m.acquireSessionProcess(ctx, job)
	if err != nil {
		return err
	}
	return m.setSessionConfig(ctx, job, id, value)
}

func (m *Manager) setSessionConfig(ctx context.Context, job *jobState, id, value string) error {
	job.sendMu.Lock()
	defer job.sendMu.Unlock()
	job.mu.RLock()
	optionIndex := slices.IndexFunc(job.agentSession.ConfigOptions, func(option sessionevents.AgentConfigOption) bool { return option.ID == id })
	valid := optionIndex >= 0 && slices.ContainsFunc(job.agentSession.ConfigOptions[optionIndex].Options, func(option sessionevents.AgentConfigValue) bool { return option.Value == value })
	modelChange := valid && job.agentSession.ConfigOptions[optionIndex].Category == "model" && job.agentSession.ConfigOptions[optionIndex].CurrentValue != value
	modelConfig := valid && job.agentSession.ConfigOptions[optionIndex].Category == "model_config"
	job.mu.RUnlock()
	if !valid {
		return fmt.Errorf("agent did not advertise config option %q with value %q", id, value)
	}
	if job.turnDone() != nil && !modelConfig {
		return fmt.Errorf("wait for the active turn before changing agent settings")
	}
	if modelChange && agentPolicyForAgent(job.ACPAgent).modelMetaKey != "" {
		return fmt.Errorf("agent %q drops Jaz's instructions when its model changes mid-session; start a new thread to use %s", CanonicalAgentName(job.ACPAgent), value)
	}
	peer := m.peer(job.ID)
	if peer == nil {
		return fmt.Errorf("agent connection is unavailable")
	}
	raw, err := setSessionConfigOption(ctx, peer, acpschema.SessionID(job.ACPSession), id, value)
	if err != nil {
		return err
	}
	m.applySessionControls(job, raw)
	job.mu.Lock()
	job.agentSession.ConfigOptions = slices.Clone(job.agentSession.ConfigOptions)
	for i := range job.agentSession.ConfigOptions {
		option := &job.agentSession.ConfigOptions[i]
		if option.ID == id {
			effective := option.CurrentValue
			option.UserValue = &effective
			if option.Category == "mode" && option.ID != job.Modes.planConfigID {
				job.Modes.userModeID = option.CurrentValue
			}
		}
	}
	job.mu.Unlock()
	m.publishAgentSession(job)
	return nil
}

// restoreConfiguredChoices reapplies the user's model and effort picks made
// since the thread last switched agent.
func restoreConfiguredChoices(cfg *AgentConfig, events []sessionevents.Event) {
	targets := map[string]*string{"model": &cfg.Model, "thought_level": &cfg.ReasoningEffort}
	start := 0
	for i, event := range events {
		if event.Type == sessionevents.TypeAgentSwitch {
			start = i + 1
		}
	}
	for _, event := range events[start:] {
		if event.AgentSession == nil {
			continue
		}
		for _, option := range event.AgentSession.ConfigOptions {
			if target := targets[option.Category]; target != nil {
				*target = ""
				if option.UserValue != nil {
					*target = *option.UserValue
				}
			}
		}
	}
}

func rememberConfiguredChoices(job *jobState, cfg AgentConfig) {
	policy := agentPolicyForAgent(job.ACPAgent)
	choices := map[string]string{"model": policy.sessionConfigModel(cfg), "thought_level": policy.sessionConfigEffort(cfg.ReasoningEffort)}
	job.mu.Lock()
	defer job.mu.Unlock()
	job.agentSession.ConfigOptions = slices.Clone(job.agentSession.ConfigOptions)
	for i := range job.agentSession.ConfigOptions {
		option := &job.agentSession.ConfigOptions[i]
		value := choices[option.Category]
		if value != "" && option.CurrentValue == value && slices.ContainsFunc(option.Options, func(item sessionevents.AgentConfigValue) bool { return item.Value == value }) {
			option.UserValue = &value
		}
	}
}

func (m *Manager) restoreSessionControls(ctx context.Context, job *jobState, events []sessionevents.Event) {
	var state *sessionevents.AgentSession
	for _, event := range events {
		if event.AgentSession != nil {
			state = event.AgentSession
		}
	}
	if state == nil {
		return
	}
	for _, option := range state.ConfigOptions {
		// Model and effort were restored before attaching the connection.
		if option.UserValue == nil || option.Category == "model" || option.Category == "thought_level" {
			continue
		}
		if err := m.setSessionConfig(ctx, job, option.ID, *option.UserValue); err != nil {
			m.log.Warn("restore agent setting", "session", job.ID, "config", option.ID, "error", err)
			job.mu.Lock()
			job.agentSession.Notices = append(job.agentSession.Notices, fmt.Sprintf("The agent could not restore %s = %s: %s", option.Name, *option.UserValue, err))
			job.mu.Unlock()
			m.publishAgentSession(job)
		}
	}
}

// CanCompact reports whether the agent of a session's live process has
// advertised its own compact.
func (m *Manager) CanCompact(sessionID string) bool {
	job, err := m.job(sessionID)
	return err == nil && job.advertises("compact")
}

func (m *Manager) supportsSessionCommand(job *jobState, name string) bool {
	job.mu.RLock()
	unknown := job.agentSession.Commands == nil
	job.mu.RUnlock()
	if unknown {
		return name == "compact" && AgentSupportsCompact(job.ACPAgent)
	}
	return job.advertises(name)
}

func (j *jobState) advertises(name string) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return slices.ContainsFunc(j.agentSession.Commands, func(command sessionevents.AgentCommand) bool {
		return command.Name == name
	})
}
