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

	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
)

const agentMethodSessionSetModel = "session/set_model"
const sessionConfigModel = "model"
const sessionConfigReasoningEffort = "reasoning_effort"
const claudeSessionConfigEffort = "effort"
const claudeReasoningEffortUltracode = "ultracode"

type ReasoningEffortOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type AgentOptions struct {
	ReasoningEfforts []ReasoningEffortOption `json:"reasoning_efforts"`
	Models           []modelcatalog.Model    `json:"models,omitempty"`
	Local            bool                    `json:"local"`
	ProviderMode     string                  `json:"provider_mode,omitempty"`
	ModelProviderIDs []string                `json:"model_provider_ids,omitempty"`
	AuthProviderID   string                  `json:"auth_provider_id,omitempty"`
	RequiresCommand  bool                    `json:"requires_command"`
	SupportsAuth     bool                    `json:"supports_auth"`
}

type setSessionModelRequest struct {
	SessionID acpschema.SessionID `json:"sessionId"`
	ModelID   string              `json:"modelId"`
}

type modelValidationKind int

const (
	modelValidationNone modelValidationKind = iota
	modelValidationClaude
)

type agentPolicy struct {
	modelConfigID       string
	effortConfigID      string
	effortInModelSuffix bool
	providerInLaunch    bool
	modelValidationKind modelValidationKind
	effortOptions       []ReasoningEffortOption
	ultracodeSetting    bool
}

var baseReasoningEffortOptions = []ReasoningEffortOption{
	{Value: "", Label: "Default"},
	{Value: "minimal", Label: "Minimal"},
	{Value: "low", Label: "Low"},
	{Value: "medium", Label: "Medium"},
	{Value: "high", Label: "High"},
	{Value: "xhigh", Label: "Extra high"},
}

var claudeReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), baseReasoningEffortOptions...),
	ReasoningEffortOption{Value: "max", Label: "Max"},
	ReasoningEffortOption{Value: claudeReasoningEffortUltracode, Label: "Ultracode"},
)

var openCodeReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), baseReasoningEffortOptions...),
	ReasoningEffortOption{Value: "max", Label: "Max"},
)

var antigravityReasoningEffortOptions = []ReasoningEffortOption{
	{Value: "", Label: "Default"},
	{Value: "minimal", Label: "Minimal"},
	{Value: "low", Label: "Low"},
	{Value: "medium", Label: "Medium"},
	{Value: "high", Label: "High"},
}

func agentPolicyForAgent(agentName string) agentPolicy {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case AgentClaude:
		return agentPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      claudeSessionConfigEffort,
			modelValidationKind: modelValidationClaude,
			effortOptions:       claudeReasoningEffortOptions,
			ultracodeSetting:    true,
		}
	case AgentCodex:
		return agentPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      sessionConfigReasoningEffort,
			effortInModelSuffix: true,
			providerInLaunch:    true,
			modelValidationKind: modelValidationNone,
			effortOptions:       baseReasoningEffortOptions,
		}
	case AgentGrok:
		return agentPolicy{
			modelValidationKind: modelValidationNone,
			effortOptions:       baseReasoningEffortOptions,
		}
	case AgentOpenCode:
		return agentPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      claudeSessionConfigEffort,
			modelValidationKind: modelValidationNone,
			effortOptions:       openCodeReasoningEffortOptions,
		}
	case AgentAntigravity:
		return agentPolicy{
			modelConfigID:       sessionConfigModel,
			effortConfigID:      sessionConfigReasoningEffort,
			modelValidationKind: modelValidationNone,
			effortOptions:       antigravityReasoningEffortOptions,
		}
	default:
		return agentPolicy{
			effortConfigID:      sessionConfigReasoningEffort,
			modelValidationKind: modelValidationNone,
			effortOptions:       baseReasoningEffortOptions,
		}
	}
}

func (p agentPolicy) usesModelConfigOption() bool {
	return p.modelConfigID != ""
}

func (p agentPolicy) reasoningEffortConfigID() string {
	return p.effortConfigID
}

func (p agentPolicy) usesReasoningEffortConfigOption() bool {
	return p.effortConfigID != ""
}

func (p agentPolicy) effortEncodedInModel(model string) bool {
	return p.effortInModelSuffix && modelHasReasoningEffort(model)
}

func (p agentPolicy) sessionConfigModel(cfg AgentConfig) string {
	if p.providerInLaunch {
		return cfg.ProviderNativeModel()
	}
	return cfg.ProviderQualifiedModel()
}

func (p agentPolicy) reasoningEffortOptions() []ReasoningEffortOption {
	return append([]ReasoningEffortOption(nil), p.effortOptions...)
}

func (p agentPolicy) normalizeReasoningEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "none" {
		value = ""
	}
	for _, option := range p.effortOptions {
		if option.Value == value {
			return value, nil
		}
	}
	return "", fmt.Errorf("unknown reasoning effort %q; valid values are %s", value, strings.Join(reasoningEffortValues(p.effortOptions), ", "))
}

func (p agentPolicy) sessionConfigEffort(value string) string {
	effort, err := p.normalizeReasoningEffort(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	if p.ultracodeSetting && effort == claudeReasoningEffortUltracode {
		return "xhigh"
	}
	return effort
}

func (p agentPolicy) mergeSessionMeta(meta map[string]any, effort string) map[string]any {
	normalized, err := p.normalizeReasoningEffort(effort)
	if err != nil || !p.ultracodeSetting || normalized != claudeReasoningEffortUltracode {
		return meta
	}
	if meta == nil {
		meta = map[string]any{}
	}
	claudeCode := nestedMap(meta, "claudeCode")
	options := nestedMap(claudeCode, "options")
	settings := nestedMap(options, "settings")
	settings[claudeReasoningEffortUltracode] = true
	return meta
}

func reasoningEffortValues(options []ReasoningEffortOption) []string {
	values := []string{"none"}
	for _, option := range options {
		if option.Value != "" {
			values = append(values, option.Value)
		}
	}
	return values
}

func nestedMap(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

type acpSessionInfo struct {
	response      acpschema.NewSessionResponse
	modelState    sessionModelState
	configOptions sessionConfigOptionsState
}

type sessionModelState struct {
	exact map[string]struct{}
	base  map[string]struct{}
}

type sessionConfigOptionsState struct {
	configOptionsPresent bool
	modelOptions         []string
	effortConfigPresent  bool
	effortConfigID       string
	effortOptions        []string
	effortPriority       int
}

// setConfiguredSessionModel applies the configured model and returns the
// agent's raw response: it carries refreshed config options (e.g. the effort
// levels valid for the newly selected model).
func (m *Manager) setConfiguredSessionModel(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, rawModel string, state sessionModelState) (json.RawMessage, error) {
	policy := agentPolicyForAgent(agentName)
	model := configuredSessionModel(rawModel)
	if policy.modelValidationKind == modelValidationClaude {
		model = state.resolveAdvertised(model)
	}
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

func (m *Manager) setConfiguredReasoningEffort(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, effort, configID string) error {
	effort, err := NormalizeAgentReasoningEffort(agentName, effort)
	if err != nil {
		return err
	}
	if effort == "" {
		return nil
	}
	policy := agentPolicyForAgent(agentName)
	if strings.TrimSpace(configID) == "" {
		if !policy.usesReasoningEffortConfigOption() {
			return nil
		}
		configID = policy.reasoningEffortConfigID()
	}
	if strings.TrimSpace(configID) == "" {
		return nil
	}
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

func (m *Manager) configuredModeState(
	ctx context.Context,
	peer *jsonrpc.Peer,
	agentName string,
	session acpSessionInfo,
	cfg AgentConfig,
) (ModeState, error) {
	policy := agentPolicyForAgent(agentName)
	effort := policy.sessionConfigEffort(cfg.ReasoningEffort)
	model := policy.sessionConfigModel(cfg)
	if _, handled, err := resolveGrokStartupConfig(agentName, cfg); err != nil {
		return ModeState{}, err
	} else if handled {
		return m.initializeModeState(ctx, peer, agentName, session.response)
	}
	modelRaw, err := m.setConfiguredSessionModel(ctx, peer, agentName, session.response.SessionID, model, session.modelState)
	if err != nil {
		return ModeState{}, err
	}
	if !policy.effortEncodedInModel(model) {
		options := session.configOptions
		activeModelOptions := false
		if refreshed := parseSessionConfigOptions(modelRaw); refreshed.configOptionsPresent {
			options = refreshed
			activeModelOptions = true
		}
		if err := m.applyConfiguredReasoningEffort(
			ctx,
			peer,
			agentName,
			session.response.SessionID,
			model,
			effort,
			options,
			activeModelOptions,
		); err != nil {
			return ModeState{}, err
		}
	}
	return m.initializeModeState(ctx, peer, agentName, session.response)
}

func (m *Manager) applyConfiguredReasoningEffort(
	ctx context.Context,
	peer *jsonrpc.Peer,
	agentName string,
	sessionID acpschema.SessionID,
	model string,
	effort string,
	options sessionConfigOptionsState,
	activeModelOptions bool,
) error {
	if effort == "" {
		return nil
	}
	if !agentPolicyForAgent(agentName).usesReasoningEffortConfigOption() && options.effortConfigID == "" {
		return nil
	}
	if activeModelOptions && !options.effortConfigPresent {
		return nil
	}
	if options.effortConfigPresent && !configOptionValueAvailable(options.effortOptions, effort) {
		return fmt.Errorf(
			"set acp agent %q reasoning effort %q for model %q: the active model did not advertise that reasoning effort; advertised values: %s",
			agentName,
			effort,
			configuredSessionModel(model),
			configOptionValuesForError(options.effortOptions),
		)
	}
	return m.setConfiguredReasoningEffort(ctx, peer, agentName, sessionID, effort, options.effortConfigID)
}

// parseEffortOptions extracts the advertised reasoning effort values from a
// session/new or set-config response's configOptions.
func parseEffortOptions(raw json.RawMessage) []string {
	return parseSessionConfigOptions(raw).effortOptions
}

func parseSessionConfigOptions(raw json.RawMessage) sessionConfigOptionsState {
	var envelope map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &envelope) != nil {
		return sessionConfigOptionsState{}
	}
	rawOptions, ok := envelope["configOptions"]
	if !ok {
		return sessionConfigOptionsState{}
	}
	state := sessionConfigOptionsState{configOptionsPresent: true}
	var options []struct {
		ID       string          `json:"id"`
		Category string          `json:"category"`
		Options  json.RawMessage `json:"options"`
	}
	if json.Unmarshal(rawOptions, &options) != nil {
		return state
	}
	for _, option := range options {
		category := strings.TrimSpace(option.Category)
		switch {
		case category == string(acpschema.SessionConfigOptionCategoryModel) || option.ID == sessionConfigModel:
			state.modelOptions = parseConfigOptionValues(option.Options)
		case category == string(acpschema.SessionConfigOptionCategoryThoughtLevel) || option.ID == claudeSessionConfigEffort || option.ID == sessionConfigReasoningEffort:
			priority := 1
			if category == string(acpschema.SessionConfigOptionCategoryThoughtLevel) {
				priority = 2
			}
			state.effortConfigPresent = true
			if priority > state.effortPriority {
				state.effortConfigID = option.ID
				state.effortOptions = parseConfigOptionValues(option.Options)
				state.effortPriority = priority
			}
		}
	}
	return state
}

func configOptionValueAvailable(options []string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, option := range options {
		if strings.ToLower(strings.TrimSpace(option)) == value {
			return true
		}
	}
	return false
}

func configOptionValuesForError(options []string) string {
	if len(options) == 0 {
		return "none"
	}
	values := make([]string, 0, len(options))
	for _, option := range options {
		if option = strings.TrimSpace(option); option != "" {
			values = append(values, option)
		}
	}
	if len(values) == 0 {
		return "none"
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func parseSessionModelState(raw json.RawMessage) sessionModelState {
	var resp struct {
		Models struct {
			AvailableModels []struct {
				ModelID string `json:"modelId"`
			} `json:"availableModels"`
		} `json:"models"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &resp) != nil {
		return sessionModelState{}
	}
	state := sessionModelState{}
	for _, model := range resp.Models.AvailableModels {
		state.addExact(model.ModelID)
	}
	configOptions := parseSessionConfigOptions(raw)
	for _, value := range configOptions.modelOptions {
		state.addBase(value)
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

func (p agentPolicy) validateConfiguredSessionModel(agentName, rawModel, effectiveModel string, state sessionModelState) error {
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
	default:
		return nil
	}
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

// resolveAdvertised maps a configured Claude model onto the spelling the agent
// currently advertises. Claude Code flips the "[1m]" context tag on a model
// between restarts (a model is bare while it is the active selection and tagged
// otherwise), so a static catalog value alternates in and out of validity. When
// the literal value is not advertised, fall back to matching by context-tag
// base and return the advertised spelling so set_config_option gets a value the
// agent accepts.
func (s sessionModelState) resolveAdvertised(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || s.empty() {
		return model
	}
	if _, ok := s.exact[model]; ok {
		return model
	}
	if _, ok := s.base[model]; ok {
		return model
	}
	base := contextTagBase(model)
	for adv := range s.exact {
		if contextTagBase(adv) == base {
			return adv
		}
	}
	for adv := range s.base {
		if contextTagBase(adv) == base {
			return adv
		}
	}
	return model
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
		configOptions: parseSessionConfigOptions(raw),
	}
}

func configuredReasoningEffort(value string) string {
	effort, err := provider.NormalizeReasoningEffort(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return effort
}

func AgentOptionsForConfig(name string, cfg AgentConfig) AgentOptions {
	options := AgentOptions{
		ReasoningEfforts: agentPolicyForAgent(CanonicalAgentName(name)).reasoningEffortOptions(),
	}
	options.Local = cfg.Local
	options.ProviderMode = strings.TrimSpace(cfg.ProviderMode)
	options.AuthProviderID = strings.TrimSpace(cfg.AuthProviderID)
	options.RequiresCommand = cfg.RequiresCommand()
	options.SupportsAuth = cfg.SupportsAuth()
	return options
}

func NormalizeAgentReasoningEffort(agentName, value string) (string, error) {
	return agentPolicyForAgent(CanonicalAgentName(agentName)).normalizeReasoningEffort(value)
}

func configuredAgentReasoningEffort(agentName, value string) string {
	effort, err := NormalizeAgentReasoningEffort(agentName, value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return effort
}

func configuredSessionModel(rawModel string) string {
	return strings.TrimSpace(rawModel)
}

func contextTagBase(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasSuffix(model, "]") {
		if i := strings.LastIndex(model, "["); i > 0 {
			return strings.TrimSpace(model[:i])
		}
	}
	return model
}

func modelHasReasoningEffort(model string) bool {
	i := strings.LastIndex(model, "/")
	if i < 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(model[i+1:])) {
	case "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}
