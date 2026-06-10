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

	"github.com/wins/jaz/backend/internal/provider"
)

const agentMethodSessionSetModel = "session/set_model"
const sessionConfigModel = "model"
const sessionConfigReasoningEffort = "reasoning_effort"
const claudeSessionConfigEffort = "effort"

type setSessionModelRequest struct {
	SessionID acpschema.SessionID `json:"sessionId"`
	ModelID   string              `json:"modelId"`
}

type modelValidationKind int

const (
	modelValidationNone modelValidationKind = iota
	modelValidationClaude
	modelValidationCodex
)

type agentModelPolicy struct {
	modelConfigID       string
	effortConfigID      string
	effortInModelSuffix bool
	modelValidationKind modelValidationKind
}

func modelPolicyForAgent(agentName string) agentModelPolicy {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case AgentClaude:
		return agentModelPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      claudeSessionConfigEffort,
			modelValidationKind: modelValidationClaude,
		}
	case AgentCodex:
		return agentModelPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      sessionConfigReasoningEffort,
			effortInModelSuffix: true,
			modelValidationKind: modelValidationCodex,
		}
	default:
		return agentModelPolicy{
			effortConfigID:      sessionConfigReasoningEffort,
			modelValidationKind: modelValidationNone,
		}
	}
}

func (p agentModelPolicy) usesModelConfigOption() bool {
	return p.modelConfigID != ""
}

func (p agentModelPolicy) reasoningEffortConfigID() string {
	return p.effortConfigID
}

func (p agentModelPolicy) effortEncodedInModel(model string) bool {
	return p.effortInModelSuffix && modelHasReasoningEffort(model)
}

type acpSessionInfo struct {
	response      acpschema.NewSessionResponse
	modelState    sessionModelState
	effortOptions []string
}

type sessionModelState struct {
	exact map[string]struct{}
	base  map[string]struct{}
}

// setConfiguredSessionModel applies the configured model and returns the
// agent's raw response: it carries refreshed config options (e.g. the effort
// levels valid for the newly selected model).
func (m *Manager) setConfiguredSessionModel(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, rawModel string, state sessionModelState) (json.RawMessage, error) {
	policy := modelPolicyForAgent(agentName)
	model := configuredSessionModel(rawModel)
	if err := policy.validateConfiguredSessionModel(agentName, rawModel, model, state); err != nil {
		return nil, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, nil
	}
	if policy.usesModelConfigOption() {
		raw, err := peer.Call(ctx, acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
			SessionID: sessionID,
			ConfigID:  acpschema.SessionConfigID(policy.modelConfigID),
			Value:     acpschema.SessionConfigValueID(model),
		})
		if err == nil {
			return raw, nil
		}
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
			return nil, fmt.Errorf("set acp agent %q model %q: session/set_config_option is not supported; clear the model in Settings > Agents or pass the model through that agent's args or env", agentName, model)
		}
		return nil, fmt.Errorf("set acp agent %q model %q: %w", agentName, model, err)
	}
	raw, err := peer.Call(ctx, agentMethodSessionSetModel, setSessionModelRequest{
		SessionID: sessionID,
		ModelID:   model,
	})
	if err == nil {
		return raw, nil
	}
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
		return nil, fmt.Errorf("set acp agent %q model %q: session/set_model is not supported; clear the model in Settings > Agents or pass the model through that agent's args or env", agentName, model)
	}
	return nil, fmt.Errorf("set acp agent %q model %q: %w", agentName, model, err)
}

func (m *Manager) setConfiguredReasoningEffort(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, effort string) error {
	effort, err := provider.NormalizeReasoningEffort(effort)
	if err != nil {
		return err
	}
	if effort == "" {
		return nil
	}
	configID := modelPolicyForAgent(agentName).reasoningEffortConfigID()
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
	policy := modelPolicyForAgent(agentName)
	effort, err := provider.NormalizeReasoningEffort(cfg.ReasoningEffort)
	if err != nil {
		return ModeState{}, err
	}
	modelRaw, err := m.setConfiguredSessionModel(ctx, peer, agentName, session.response.SessionID, cfg.Model, session.modelState)
	if err != nil {
		return ModeState{}, err
	}
	if !policy.effortEncodedInModel(cfg.Model) {
		// Valid effort levels depend on the active model (claude's sonnet
		// drops xhigh); switching models refreshes the advertised list.
		advertised := session.effortOptions
		if updated := parseEffortOptions(modelRaw); len(updated) > 0 {
			advertised = updated
		}
		if clamped := clampReasoningEffort(effort, advertised); clamped != effort {
			m.log.Info("clamped acp reasoning effort to the active model",
				"agent", agentName, "model", cfg.Model, "requested", effort,
				"using", firstNonEmpty(clamped, "agent default"))
			effort = clamped
		}
		if err := m.setConfiguredReasoningEffort(ctx, peer, agentName, session.response.SessionID, effort); err != nil {
			return ModeState{}, err
		}
	}
	return m.initializeModeState(ctx, peer, session.response)
}

var effortLadder = []string{"minimal", "low", "medium", "high", "xhigh"}

// clampReasoningEffort fits the configured effort to the agent's advertised
// levels: the nearest weaker level wins, then the nearest stronger. Empty
// result means skip the option and let the agent use its default; an unknown
// advertisement leaves the effort untouched.
func clampReasoningEffort(effort string, advertised []string) string {
	if effort == "" || len(advertised) == 0 {
		return effort
	}
	available := make(map[string]struct{}, len(advertised))
	for _, value := range advertised {
		available[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	if _, ok := available[effort]; ok {
		return effort
	}
	index := -1
	for i, level := range effortLadder {
		if level == effort {
			index = i
			break
		}
	}
	if index < 0 {
		return effort
	}
	for i := index - 1; i >= 0; i-- {
		if _, ok := available[effortLadder[i]]; ok {
			return effortLadder[i]
		}
	}
	for i := index + 1; i < len(effortLadder); i++ {
		if _, ok := available[effortLadder[i]]; ok {
			return effortLadder[i]
		}
	}
	return ""
}

// parseEffortOptions extracts the advertised reasoning effort values from a
// session/new or set-config response's configOptions.
func parseEffortOptions(raw json.RawMessage) []string {
	var resp struct {
		ConfigOptions []struct {
			ID      string          `json:"id"`
			Options json.RawMessage `json:"options"`
		} `json:"configOptions"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &resp) != nil {
		return nil
	}
	for _, option := range resp.ConfigOptions {
		if option.ID == claudeSessionConfigEffort || option.ID == sessionConfigReasoningEffort {
			return parseConfigOptionValues(option.Options)
		}
	}
	return nil
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

func (p agentModelPolicy) validateConfiguredSessionModel(agentName, rawModel, effectiveModel string, state sessionModelState) error {
	if strings.TrimSpace(rawModel) == "" || state.empty() {
		return nil
	}
	switch p.modelValidationKind {
	case modelValidationClaude:
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
	case modelValidationCodex:
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
		response:      session,
		modelState:    parseSessionModelState(raw),
		effortOptions: parseEffortOptions(raw),
	}
}

func configuredReasoningEffort(value string) string {
	effort, err := provider.NormalizeReasoningEffort(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return effort
}

func configuredSessionModel(rawModel string) string {
	return strings.TrimSpace(rawModel)
}

func modelHasReasoningEffort(model string) bool {
	i := strings.LastIndex(model, "/")
	if i < 0 {
		return false
	}
	effort, err := provider.NormalizeReasoningEffort(model[i+1:])
	return err == nil && effort != ""
}
