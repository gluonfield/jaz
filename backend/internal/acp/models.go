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
const sessionConfigCollaborationMode = "collaboration_mode"
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
	Models           []AgentModel            `json:"models,omitempty"`
	DefaultModel     string                  `json:"default_model,omitempty"`
	DefaultProvider  string                  `json:"default_model_provider,omitempty"`
	DefaultEffort    string                  `json:"default_reasoning_effort,omitempty"`
	Local            bool                    `json:"local"`
	ProviderMode     string                  `json:"provider_mode,omitempty"`
	ModelProviderIDs []string                `json:"model_provider_ids,omitempty"`
	AuthProviderID   string                  `json:"auth_provider_id,omitempty"`
	SupportsAuth     bool                    `json:"supports_auth"`
	FastModeConfigID string                  `json:"fast_mode_config_id,omitempty"`
	FastModeModels   []string                `json:"fast_mode_models,omitempty"`
}

type setSessionModelRequest struct {
	SessionID acpschema.SessionID `json:"sessionId"`
	ModelID   string              `json:"modelId"`
}

// unadvertisedModelPolicy decides what happens when the agent does not offer
// the configured model.
type unadvertisedModelPolicy int

const (
	// unadvertisedModelSent leaves the model id to the agent to accept or refuse.
	unadvertisedModelSent unadvertisedModelPolicy = iota
	// unadvertisedModelSkipped keeps the agent's own default for a model saved
	// earlier. Codex model access follows the signed-in account's plan, so a
	// model that worked yesterday can disappear without anyone touching the
	// settings; that must not cost the session. A model the request names is
	// refused, so whoever named it learns it is not on offer.
	unadvertisedModelSkipped
	// unadvertisedModelRejected fails the session rather than running a model
	// other than the configured one.
	unadvertisedModelRejected
)

type agentPolicy struct {
	modelConfigID           string
	modelMetaKey            string
	effortConfigID          string
	effortInModelSuffix     bool
	providerInLaunch        bool
	modelConfiguredAtLaunch bool
	systemPromptAtLaunch    bool
	promptPersistsOnRestore bool
	materializesOnPrompt    bool
	unadvertisedModel       unadvertisedModelPolicy
	resolvesContextTag      bool
	effortOptions           []ReasoningEffortOption
	ultracodeSetting        bool
	systemPromptAppendMeta  bool
	promptSuggestions       bool
}

var baseReasoningEffortOptions = []ReasoningEffortOption{
	{Value: "", Label: "Default"},
	{Value: "minimal", Label: "Minimal"},
	{Value: "low", Label: "Low"},
	{Value: "medium", Label: "Medium"},
	{Value: "high", Label: "High"},
	{Value: "xhigh", Label: "Extra high"},
}

var grokReasoningEffortOptions = []ReasoningEffortOption{
	{Value: "", Label: "Default"},
	{Value: "low", Label: "Low"},
	{Value: "medium", Label: "Medium"},
	{Value: "high", Label: "High"},
	{Value: "xhigh", Label: "Extra high"},
}

var claudeReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), baseReasoningEffortOptions...),
	ReasoningEffortOption{Value: "max", Label: "Max"},
	ReasoningEffortOption{Value: claudeReasoningEffortUltracode, Label: "Ultracode"},
)

var codexReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), baseReasoningEffortOptions...),
	ReasoningEffortOption{Value: "max", Label: "Max"},
	ReasoningEffortOption{Value: "ultra", Label: "Ultra"},
)

var openCodeReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), baseReasoningEffortOptions...),
	ReasoningEffortOption{Value: "max", Label: "Max"},
)

var museReasoningEffortOptions = append(append([]ReasoningEffortOption(nil), codexReasoningEffortOptions...),
	ReasoningEffortOption{Value: "none", Label: "None"},
)

func agentPolicyForAgent(agentName string) agentPolicy {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case AgentClaude:
		return agentPolicy{
			modelConfigID:        sessionConfigModel,
			effortConfigID:       claudeSessionConfigEffort,
			materializesOnPrompt: true,
			unadvertisedModel:    unadvertisedModelRejected,
			resolvesContextTag:   true,
			effortOptions:        claudeReasoningEffortOptions,
			ultracodeSetting:     true,
			promptSuggestions:    true,
		}
	case AgentCodex:
		return agentPolicy{
			modelConfigID:           sessionConfigModel,
			effortConfigID:          sessionConfigReasoningEffort,
			effortInModelSuffix:     true,
			providerInLaunch:        true,
			modelConfiguredAtLaunch: true,
			systemPromptAtLaunch:    true,
			promptPersistsOnRestore: true,
			materializesOnPrompt:    true,
			unadvertisedModel:       unadvertisedModelSkipped,
			effortOptions:           codexReasoningEffortOptions,
		}
	case AgentKimi:
		return agentPolicy{
			modelConfigID:          sessionConfigModel,
			systemPromptAppendMeta: true,
		}
	case AgentGrok:
		// Grok rebuilds its system prompt without session rules when the model
		// changes after session/new, so the model rides session/new instead.
		// It cannot refuse a model sent there and silently falls back to its
		// default for ids it does not offer, so Jaz refuses them.
		return agentPolicy{
			modelMetaKey:      "modelId",
			unadvertisedModel: unadvertisedModelRejected,
			effortOptions:     grokReasoningEffortOptions,
		}
	case AgentOpenCode:
		return agentPolicy{
			modelConfigID:           sessionConfigModel,
			effortConfigID:          claudeSessionConfigEffort,
			modelConfiguredAtLaunch: true,
			effortOptions:           openCodeReasoningEffortOptions,
		}
	case AgentAntigravity:
		return agentPolicy{
			modelConfigID: sessionConfigModel,
		}
	case AgentMuse:
		return agentPolicy{
			modelConfigID:        sessionConfigModel,
			effortConfigID:       sessionConfigReasoningEffort,
			systemPromptAtLaunch: true,
			effortOptions:        museReasoningEffortOptions,
		}
	default:
		return agentPolicy{
			effortConfigID: sessionConfigReasoningEffort,
			effortOptions:  baseReasoningEffortOptions,
		}
	}
}

// KeepsStartPrompt reports whether agent keeps the prompt its thread started
// with in provider history, whatever prompt a restored session is given.
func KeepsStartPrompt(agent string) bool {
	return agentPolicyForAgent(agent).promptPersistsOnRestore
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
	_, _, suffixed := cutReasoningEffort(model)
	return p.effortInModelSuffix && suffixed
}

func (p agentPolicy) sessionConfigModel(cfg AgentConfig) string {
	if p.providerInLaunch {
		return cfg.ProviderNativeModel()
	}
	return cfg.ProviderQualifiedModel()
}

func (p agentPolicy) reasoningEffortOptions() []ReasoningEffortOption {
	if len(p.effortOptions) == 0 {
		return nil
	}
	return append([]ReasoningEffortOption(nil), p.effortOptions...)
}

func (p agentPolicy) supportsReasoningEffort(value string) bool {
	for _, option := range p.effortOptions {
		if option.Value == value {
			return true
		}
	}
	return false
}

func (p agentPolicy) normalizeReasoningEffort(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") && !p.supportsReasoningEffort("none") {
		value = ""
	}
	if len(p.effortOptions) == 0 {
		return value, nil
	}
	value = strings.ToLower(value)
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

// mergeSessionMeta adds the agent options a session of sourceType starts with.
// Sessions nobody writes to disable Claude's optional prompt prediction request.
func (p agentPolicy) mergeSessionMeta(meta map[string]any, cfg AgentConfig, sourceType string) map[string]any {
	if model := configuredSessionModel(cfg.ProviderQualifiedModel()); p.modelMetaKey != "" && model != "" {
		if meta == nil {
			meta = map[string]any{}
		}
		meta[p.modelMetaKey] = model
	}
	if p.promptSuggestions && !watchedSourceType(sourceType) {
		if meta == nil {
			meta = map[string]any{}
		}
		nestedMap(nestedMap(meta, "claudeCode"), "options")["promptSuggestions"] = false
	}
	normalized, err := p.normalizeReasoningEffort(cfg.ReasoningEffort)
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
		if option.Value != "" && option.Value != "none" {
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
	planConfigID         string
	modelOptions         []string
	currentModel         string
	defaultModel         string
	effortConfigPresent  bool
	effortConfigID       string
	effortOptions        []string
	effortPriority       int
}

// setConfiguredSessionModel checks the configured model against the agent's
// advertised models and applies it unless it already rode the launch or
// session/new; a model the agent does not offer gives way to the agent's
// default. It returns the agent's raw response: it carries refreshed config
// options (e.g. the effort levels valid for the newly selected model).
func (m *Manager) setConfiguredSessionModel(ctx context.Context, peer *jsonrpc.Peer, agentName string, session acpSessionInfo, rawModel string, requested bool, state sessionModelState) (json.RawMessage, error) {
	policy := agentPolicyForAgent(agentName)
	configured := configuredSessionModel(rawModel)
	model, err := policy.sessionModelToSend(agentName, configured, state, requested)
	if err != nil {
		return nil, err
	}
	options := session.configOptions
	switch {
	case model == "" && configured != "":
		m.log.Warn("acp agent does not offer the configured model",
			"agent", agentName, "model", configured, "current", options.currentModel,
			"default", options.defaultModel, "available", strings.Join(state.advertisedModels(), ", "))
		// A launch before Jaz had seen the agent's list may have carried the
		// model, so the session moves to the agent's default, as a switch.
		if options.currentModel == options.defaultModel {
			return nil, nil
		}
		model = options.defaultModel
	case policy.modelConfiguredAtLaunch || policy.modelMetaKey != "":
		return nil, nil
	}
	if model == "" {
		return nil, nil
	}
	sessionID := session.response.SessionID
	if policy.usesModelConfigOption() {
		raw, err := setSessionConfigOption(ctx, peer, sessionID, policy.modelConfigID, model)
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

func setSessionConfigOption(ctx context.Context, peer *jsonrpc.Peer, sessionID acpschema.SessionID, configID, value string) (json.RawMessage, error) {
	return peer.Call(ctx, acpschema.AgentMethodSessionSetConfigOption, acpschema.SetSessionConfigOptionRequest{
		SessionID: sessionID,
		ConfigID:  acpschema.SessionConfigID(configID),
		Value:     acpschema.EncodeSessionConfigValue(acpschema.SessionConfigValueID(value)),
	})
}

func (m *Manager) setConfiguredReasoningEffort(ctx context.Context, peer *jsonrpc.Peer, agentName string, sessionID acpschema.SessionID, effort, configID string) (json.RawMessage, error) {
	if effort == "" {
		return nil, nil
	}
	policy := agentPolicyForAgent(agentName)
	if strings.TrimSpace(configID) == "" {
		if !policy.usesReasoningEffortConfigOption() {
			return nil, nil
		}
		configID = policy.reasoningEffortConfigID()
	}
	if strings.TrimSpace(configID) == "" {
		return nil, nil
	}
	raw, err := setSessionConfigOption(ctx, peer, sessionID, configID, effort)
	if err == nil {
		return raw, nil
	}
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == -32601 {
		return nil, fmt.Errorf("set acp agent %q reasoning effort %q: session/set_config_option is not supported; clear the reasoning effort in Settings > Agents or pass the effort through that agent's args or env", agentName, effort)
	}
	return nil, fmt.Errorf("set acp agent %q reasoning effort %q: %w", agentName, effort, err)
}

func (m *Manager) configuredModeState(
	ctx context.Context,
	ac *agentConn,
	agentName string,
	session acpSessionInfo,
	cfg AgentConfig,
) (ModeState, error) {
	peer := ac.peer
	policy := agentPolicyForAgent(agentName)
	effort := policy.sessionConfigEffort(cfg.ReasoningEffort)
	model := policy.sessionConfigModel(cfg)
	m.rememberAdvertisedModels(agentName, cfg, session.modelState)
	// Codex lists its own models whichever provider it runs on, while another
	// provider's model rides the launch, so only its own list can judge one.
	advertised := session.modelState
	if policy.providerInLaunch && !agentOwnsModelMetadata(agentName, cfg.ModelProvider) {
		advertised = sessionModelState{}
	}
	modelRaw, err := m.setConfiguredSessionModel(ctx, peer, agentName, session, model, cfg.ModelRequested, advertised)
	if err != nil {
		return ModeState{}, err
	}
	ac.state.configResponse(session.response.SessionID, modelRaw)
	if !policy.effortEncodedInModel(model) {
		options := session.configOptions
		// session/new describes the model that rode the launch.
		activeModelOptions := policy.modelConfiguredAtLaunch && options.configOptionsPresent
		if refreshed := parseSessionConfigOptions(modelRaw); refreshed.configOptionsPresent {
			options = refreshed
			activeModelOptions = true
		}
		effortRaw, err := m.applyConfiguredReasoningEffort(
			ctx,
			peer,
			agentName,
			session.response.SessionID,
			model,
			effort,
			options,
			activeModelOptions,
		)
		if err != nil {
			return ModeState{}, err
		}
		ac.state.configResponse(session.response.SessionID, effortRaw)
	}
	return m.initializeModeState(ctx, peer, agentName, session)
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
) (json.RawMessage, error) {
	if effort == "" {
		return nil, nil
	}
	if !agentPolicyForAgent(agentName).usesReasoningEffortConfigOption() && options.effortConfigID == "" {
		return nil, fmt.Errorf("acp agent %q model %q did not advertise reasoning controls", agentName, model)
	}
	if activeModelOptions && !options.effortConfigPresent {
		return nil, nil
	}
	if options.effortConfigPresent && !configOptionValueAvailable(options.effortOptions, effort) {
		return nil, fmt.Errorf(
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

// configOptionMeta carries the value an agent recommends for one of its
// options, such as Codex's default model, through the JetBrains AIR extension
// Jaz advertises at initialize.
type configOptionMeta struct {
	Jetbrains struct {
		Air struct {
			RecommendedValue string `json:"recommendedValue"`
		} `json:"air"`
	} `json:"jetbrains"`
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
		ID           string           `json:"id"`
		Category     string           `json:"category"`
		CurrentValue string           `json:"currentValue"`
		Options      json.RawMessage  `json:"options"`
		Meta         configOptionMeta `json:"_meta"`
	}
	if json.Unmarshal(rawOptions, &options) != nil {
		return state
	}
	for _, option := range options {
		category := strings.TrimSpace(option.Category)
		switch {
		case option.ID == sessionConfigCollaborationMode:
			values := parseConfigOptionValues(option.Options)
			if configOptionValueAvailable(values, "default") && configOptionValueAvailable(values, "plan") {
				state.planConfigID = option.ID
			}
		case category == string(acpschema.SessionConfigOptionCategoryModel) || option.ID == sessionConfigModel:
			state.modelOptions = parseConfigOptionValues(option.Options)
			state.currentModel = option.CurrentValue
			state.defaultModel = option.Meta.Jetbrains.Air.RecommendedValue
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

type advertisedModelList struct {
	AvailableModels []struct {
		ModelID string `json:"modelId"`
	} `json:"availableModels"`
}

func (l advertisedModelList) state() sessionModelState {
	state := sessionModelState{}
	for _, model := range l.AvailableModels {
		state.addExact(model.ModelID)
	}
	return state
}

func parseSessionModelState(raw json.RawMessage) sessionModelState {
	var resp struct {
		Models advertisedModelList `json:"models"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &resp) != nil {
		return sessionModelState{}
	}
	state := resp.Models.state()
	// The model config option repeats the session's current model even when the
	// agent does not offer it, so it only describes what is available when the
	// agent advertised nothing else.
	if len(state.exact) == 0 {
		for _, value := range parseSessionConfigOptions(raw).modelOptions {
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

// sessionModelToSend resolves the model id to apply to a session. An empty id
// means the session keeps whichever model the agent chose for itself.
// requested says the request starting the session named the model.
func (p agentPolicy) sessionModelToSend(agentName, rawModel string, state sessionModelState, requested bool) (string, error) {
	model := configuredSessionModel(rawModel)
	if model == "" || state.empty() {
		return model, nil
	}
	if advertised, ok := state.matchAdvertised(model); ok {
		if p.resolvesContextTag {
			return advertised, nil
		}
		return model, nil
	}
	switch p.unadvertisedModel {
	case unadvertisedModelSkipped:
		if !requested {
			return "", nil
		}
		fallthrough
	case unadvertisedModelRejected:
		return "", unadvertisedModelError(agentName, model, state)
	}
	return model, nil
}

func unadvertisedModelError(agentName, model string, state sessionModelState) error {
	return fmt.Errorf("configured acp agent %q model %q is not advertised by the agent; available model ids: %s",
		agentName, model, strings.Join(state.advertisedModels(), ", "))
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
	if base, _, ok := cutReasoningEffort(model); ok {
		s.addBase(base)
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

// matchAdvertised finds the spelling the agent currently advertises for a
// configured model, ignoring the tag agents append to a model id: Claude Code
// flips the "[1m]" context tag between restarts (a model is bare while it is
// the active selection and tagged otherwise), and Codex advertises one id per
// reasoning effort ("gpt-5.6-terra[high]") while accepting the bare id.
func (s sessionModelState) matchAdvertised(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if model == "" || s.empty() {
		return "", false
	}
	if _, ok := s.exact[model]; ok {
		return model, true
	}
	if _, ok := s.base[model]; ok {
		return model, true
	}
	base := contextTagBase(model)
	for _, advertised := range []map[string]struct{}{s.exact, s.base} {
		for candidate := range advertised {
			if contextTagBase(candidate) == base {
				return candidate, true
			}
		}
	}
	return "", false
}

// advertisedModels lists the selectable model ids, stripped of the per-effort
// and per-context tags that would otherwise repeat one model many times.
func (s sessionModelState) advertisedModels() []string {
	advertised := s.exact
	if len(advertised) == 0 {
		advertised = s.base
	}
	models := make(map[string]struct{}, len(advertised))
	for model := range advertised {
		models[contextTagBase(model)] = struct{}{}
	}
	return sortedKeys(models)
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

func AgentOptionsForConfig(name string, cfg AgentConfig, root string) AgentOptions {
	options := AgentOptions{
		ReasoningEfforts: agentPolicyForAgent(CanonicalAgentName(name)).reasoningEffortOptions(),
		DefaultModel:     strings.TrimSpace(cfg.Model),
		DefaultProvider:  strings.TrimSpace(cfg.ModelProvider),
		DefaultEffort:    strings.TrimSpace(cfg.ReasoningEffort),
	}
	if options.ReasoningEfforts == nil {
		options.ReasoningEfforts = []ReasoningEffortOption{}
	}
	options.Local = cfg.Local
	options.ProviderMode = strings.TrimSpace(cfg.ProviderMode)
	options.AuthProviderID = strings.TrimSpace(cfg.AuthProviderID)
	options.SupportsAuth = cfg.SupportsAuth()
	if CanonicalAgentName(name) == AgentCodex && codexNativeOpenAIProvider(cfg.ModelProvider) {
		options.FastModeModels = codexFastModeModels(cfg, root)
		if len(options.FastModeModels) > 0 {
			options.FastModeConfigID = "fast-mode"
		}
	}
	return options
}

func NormalizeAgentReasoningEffort(agentName, value string) (string, error) {
	return agentPolicyForAgent(CanonicalAgentName(agentName)).normalizeReasoningEffort(value)
}

func DefaultAgentReasoningEffort(agentName string) string {
	switch agentName = CanonicalAgentName(agentName); agentName {
	case AgentCodex, AgentClaude, AgentOpenCode:
	default:
		return ""
	}
	if agentPolicyForAgent(agentName).supportsReasoningEffort("xhigh") {
		return "xhigh"
	}
	return ""
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

// cutReasoningEffort splits the reasoning effort off a model id that carries
// one, such as gpt-6-luna/high.
func cutReasoningEffort(model string) (base, effort string, ok bool) {
	i := strings.LastIndex(model, "/")
	if i < 0 {
		return model, "", false
	}
	switch strings.ToLower(strings.TrimSpace(model[i+1:])) {
	case "minimal", "low", "medium", "high", "xhigh", "max":
		return model[:i], model[i+1:], true
	default:
		return model, "", false
	}
}
