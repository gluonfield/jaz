package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

const agentMethodSessionSetModel = "session/set_model"
const sessionConfigModel = "model"
const sessionConfigReasoningEffort = "reasoning_effort"
const claudeSessionConfigEffort = "effort"

type setSessionModelRequest struct {
	SessionID acpschema.SessionID `json:"sessionId"`
	ModelID   string              `json:"modelId"`
}

type acpSessionInfo struct {
	response   acpschema.NewSessionResponse
	modelState sessionModelState
}

type sessionModelState struct {
	exact map[string]struct{}
	base  map[string]struct{}
}

func (m *Manager) setConfiguredSessionModel(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, rawModel string, effort string, state sessionModelState) error {
	model := configuredSessionModel(agentName, rawModel, effort)
	if err := validateConfiguredSessionModel(agentName, rawModel, model, state); err != nil {
		return err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(agentName)) == AgentClaude {
		_, err := peer.Call(ctx, acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
			SessionID: sessionID,
			ConfigID:  acpschema.SessionConfigID(sessionConfigModel),
			Value:     acpschema.SessionConfigValueID(model),
		})
		if err == nil {
			return nil
		}
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
			return fmt.Errorf("set acp agent %q model %q: session/set_config_option is not supported; clear the model in Settings > Agents or pass the model through that agent's args or env", agentName, model)
		}
		return fmt.Errorf("set acp agent %q model %q: %w", agentName, model, err)
	}
	_, err := peer.Call(ctx, agentMethodSessionSetModel, setSessionModelRequest{
		SessionID: sessionID,
		ModelID:   model,
	})
	if err == nil {
		return nil
	}
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
		return fmt.Errorf("set acp agent %q model %q: session/set_model is not supported; clear the model in Settings > Agents or pass the model through that agent's args or env", agentName, model)
	}
	return fmt.Errorf("set acp agent %q model %q: %w", agentName, model, err)
}

func (m *Manager) setConfiguredReasoningEffort(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, effort string) error {
	effort, err := normalizeSessionReasoningEffort(effort)
	if err != nil {
		return err
	}
	if effort == "" {
		return nil
	}
	configID := reasoningEffortConfigID(agentName)
	_, err = peer.Call(ctx, acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
		SessionID: sessionID,
		ConfigID:  acpschema.SessionConfigID(configID),
		Value:     acpschema.SessionConfigValueID(effort),
	})
	if err == nil {
		return nil
	}
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
		return fmt.Errorf("set acp agent %q reasoning effort %q: session/set_config_option is not supported; clear the reasoning effort in Settings > Agents or pass the effort through that agent's args or env", agentName, effort)
	}
	return fmt.Errorf("set acp agent %q reasoning effort %q: %w", agentName, effort, err)
}

func (m *Manager) configuredModeState(ctx context.Context, peer *jsonrpc.Peer, agentName string, session acpSessionInfo, cfg AgentConfig) (ModeState, error) {
	effort, err := normalizeSessionReasoningEffort(cfg.ReasoningEffort)
	if err != nil {
		return ModeState{}, err
	}
	if err := m.setConfiguredSessionModel(ctx, peer, agentName, session.response.SessionID, cfg.Model, effort, session.modelState); err != nil {
		return ModeState{}, err
	}
	if !reasoningEffortEncodedInModel(agentName, cfg.Model, effort) {
		if err := m.setConfiguredReasoningEffort(ctx, peer, agentName, session.response.SessionID, effort); err != nil {
			return ModeState{}, err
		}
	}
	return m.initializeModeState(ctx, peer, session.response)
}

func parseSessionModelState(raw json.RawMessage) sessionModelState {
	var resp struct {
		Models struct {
			AvailableModels []struct {
				ModelID string `json:"modelId"`
			} `json:"availableModels"`
		} `json:"models"`
		ConfigOptions []struct {
			ID      string          `json:"id"`
			Options json.RawMessage `json:"options"`
		} `json:"configOptions"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &resp) != nil {
		return sessionModelState{}
	}
	state := sessionModelState{}
	for _, model := range resp.Models.AvailableModels {
		state.addExact(model.ModelID)
	}
	for _, option := range resp.ConfigOptions {
		if option.ID != "model" {
			continue
		}
		for _, value := range parseConfigOptionValues(option.Options) {
			state.addBase(value)
		}
	}
	return state
}

func parseConfigOptionValues(raw json.RawMessage) []string {
	var flat []struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &flat) == nil {
		values := make([]string, 0, len(flat))
		for _, option := range flat {
			if option.Value != "" {
				values = append(values, option.Value)
			}
		}
		if len(values) > 0 {
			return values
		}
	}
	var grouped []struct {
		Options []struct {
			Value string `json:"value"`
		} `json:"options"`
	}
	if json.Unmarshal(raw, &grouped) != nil {
		return nil
	}
	var values []string
	for _, group := range grouped {
		for _, option := range group.Options {
			values = append(values, option.Value)
		}
	}
	return values
}

func validateConfiguredSessionModel(agentName, rawModel, effectiveModel string, state sessionModelState) error {
	if strings.TrimSpace(rawModel) == "" || state.empty() {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case AgentClaude:
		if state.hasExact(effectiveModel) || state.hasBase(effectiveModel) {
			return nil
		}
		available := state.availableExact()
		if len(available) == 0 {
			available = state.availableBases()
		}
		if len(available) == 0 {
			return nil
		}
		return fmt.Errorf("configured acp agent %q model %q is not advertised by the agent; available model ids: %s", agentName, effectiveModel, strings.Join(available, ", "))
	case AgentCodex:
	default:
		return nil
	}
	if modelHasReasoningEffort(effectiveModel) && len(state.exact) > 0 {
		if state.hasExact(effectiveModel) {
			return nil
		}
		return fmt.Errorf("configured acp agent %q model %q is not advertised by the agent; available model ids: %s", agentName, effectiveModel, strings.Join(state.availableExact(), ", "))
	}
	if state.hasBase(rawModel) {
		return nil
	}
	available := state.availableBases()
	if len(available) == 0 {
		available = state.availableExact()
	}
	if len(available) == 0 {
		return nil
	}
	return fmt.Errorf("configured acp agent %q model %q is not advertised by the agent; available models: %s", agentName, effectiveModel, strings.Join(available, ", "))
}

func (s *sessionModelState) addExact(model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	if s.exact == nil {
		s.exact = make(map[string]struct{})
	}
	s.exact[model] = struct{}{}
	if modelHasReasoningEffort(model) {
		s.addBase(model[:strings.LastIndex(model, "/")])
	}
}

func (s *sessionModelState) addBase(model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	if s.base == nil {
		s.base = make(map[string]struct{})
	}
	s.base[model] = struct{}{}
}

func (s sessionModelState) empty() bool {
	return len(s.exact) == 0 && len(s.base) == 0
}

func (s sessionModelState) hasExact(model string) bool {
	_, ok := s.exact[strings.TrimSpace(model)]
	return ok
}

func (s sessionModelState) hasBase(model string) bool {
	model = strings.TrimSpace(model)
	if modelHasReasoningEffort(model) {
		model = model[:strings.LastIndex(model, "/")]
	}
	_, ok := s.base[model]
	return ok
}

func (s sessionModelState) availableBases() []string {
	return sortedKeys(s.base)
}

func (s sessionModelState) availableExact() []string {
	return sortedKeys(s.exact)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newACPSessionInfo(raw json.RawMessage, session acpschema.NewSessionResponse) acpSessionInfo {
	return acpSessionInfo{
		response:   session,
		modelState: parseSessionModelState(raw),
	}
}

func configuredReasoningEffort(value string) string {
	effort, err := normalizeSessionReasoningEffort(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return effort
}

func configuredSessionModel(agentName, rawModel, effort string) string {
	model := strings.TrimSpace(rawModel)
	if model == "" || effort == "" || strings.ToLower(strings.TrimSpace(agentName)) != AgentCodex || modelHasReasoningEffort(model) {
		return model
	}
	return model + "/" + effort
}

func reasoningEffortEncodedInModel(agentName, model, effort string) bool {
	return strings.ToLower(strings.TrimSpace(agentName)) == AgentCodex && strings.TrimSpace(model) != "" && effort != ""
}

func reasoningEffortConfigID(agentName string) string {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case AgentClaude:
		return claudeSessionConfigEffort
	default:
		return sessionConfigReasoningEffort
	}
}

func normalizeSessionReasoningEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none":
		return "", nil
	case "minimal", "low", "medium", "high", "xhigh":
		return value, nil
	default:
		return "", fmt.Errorf("unknown acp reasoning effort %q; valid values are none, minimal, low, medium, high, xhigh", value)
	}
}

func modelHasReasoningEffort(model string) bool {
	i := strings.LastIndex(model, "/")
	if i < 0 {
		return false
	}
	effort, err := normalizeSessionReasoningEffort(model[i+1:])
	return err == nil && effort != ""
}
