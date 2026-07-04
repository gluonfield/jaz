package settings

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	MemorySettingsNamespace = "memory"
	MemorySettingsKey       = "settings"
)

type MemorySettings struct {
	Enabled         bool   `json:"enabled"`
	Agent           string `json:"agent,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type memorySettingsStorage struct {
	Enabled           bool   `json:"enabled"`
	Agent             string `json:"agent,omitempty"`
	Model             string `json:"model,omitempty"`
	ReasoningEffort   string `json:"reasoning_effort,omitempty"`
	LegacyDreamAgent  string `json:"dream_agent,omitempty"`
	LegacySearchAgent string `json:"search_agent,omitempty"`
}

func DefaultMemorySettings() MemorySettings {
	return MemorySettings{Enabled: true}
}

func (m MemorySettings) WorkerModel(defaults AgentDefaults) string {
	if m.Model != "" {
		return m.Model
	}
	return WorkerAgentModel(m.Agent, defaults)
}

func (m MemorySettings) WorkerReasoningEffort() string {
	if m.ReasoningEffort != "" {
		return m.ReasoningEffort
	}
	return WorkerAgentReasoningEffort(m.Agent)
}

func LoadMemorySettings(store storage.SettingsStorage) (MemorySettings, error) {
	setting, err := store.LoadSetting(MemorySettingsNamespace, MemorySettingsKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return DefaultMemorySettings(), nil
	}
	if err != nil {
		return MemorySettings{}, err
	}
	var stored memorySettingsStorage
	if err := json.Unmarshal(setting.Value, &stored); err != nil {
		return MemorySettings{}, err
	}
	return MemorySettings{
		Enabled:         stored.Enabled,
		Agent:           firstMemoryAgent(stored.Agent, stored.LegacySearchAgent, stored.LegacyDreamAgent),
		Model:           strings.TrimSpace(stored.Model),
		ReasoningEffort: strings.TrimSpace(stored.ReasoningEffort),
	}, nil
}

func SaveMemorySettings(store storage.SettingsStorage, settings MemorySettings) (MemorySettings, error) {
	settings.Agent = strings.TrimSpace(settings.Agent)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.ReasoningEffort = strings.TrimSpace(settings.ReasoningEffort)
	data, err := json.Marshal(settings)
	if err != nil {
		return MemorySettings{}, err
	}
	if _, err := store.SaveSetting(MemorySettingsNamespace, MemorySettingsKey, data); err != nil {
		return MemorySettings{}, err
	}
	return settings, nil
}

func firstMemoryAgent(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// MemoryEnabled is the live gate used per turn, spawn, and request; storage
// errors fail open so a settings hiccup never silently disables memory.
func MemoryEnabled(store storage.SettingsStorage) bool {
	settings, err := LoadMemorySettings(store)
	if err != nil {
		return true
	}
	return settings.Enabled
}
