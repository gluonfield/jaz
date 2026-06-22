package settings

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	MemorySettingsNamespace = "memory"
	MemorySettingsKey       = "settings"
)

type MemorySettings struct {
	Enabled bool   `json:"enabled"`
	Agent   string `json:"agent,omitempty"`
}

type memorySettingsStorage struct {
	Enabled           bool   `json:"enabled"`
	Agent             string `json:"agent,omitempty"`
	LegacyDreamAgent  string `json:"dream_agent,omitempty"`
	LegacySearchAgent string `json:"search_agent,omitempty"`
}

func DefaultMemorySettings() MemorySettings {
	return MemorySettings{Enabled: true}
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
		Enabled: stored.Enabled,
		Agent:   firstMemoryAgent(stored.Agent, stored.LegacySearchAgent, stored.LegacyDreamAgent),
	}, nil
}

func SaveMemorySettings(store storage.SettingsStorage, settings MemorySettings) (MemorySettings, error) {
	settings.Agent = strings.TrimSpace(settings.Agent)
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

func DefaultMemoryAgent(defaults AgentDefaults) string {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude, acp.AgentOpenCode} {
		if current, ok := defaults.ACP[agent]; ok && current.Enabled {
			return agent
		}
	}
	return ""
}

func MemoryAgentModel(agent string, defaults AgentDefaults) string {
	switch acp.CanonicalAgentName(agent) {
	case acp.AgentCodex:
		return "gpt-5.4-mini"
	case acp.AgentClaude:
		return "sonnet"
	case acp.AgentGrok:
		return "grok-composer-2.5-fast"
	case acp.AgentOpenCode:
		switch strings.TrimSpace(defaults.ACP[acp.AgentOpenCode].ModelProvider) {
		case provider.ProviderOpenAI:
			return "gpt-5.4-mini"
		case "", provider.ProviderOpenRouter:
			return "openai/gpt-5.4-mini"
		default:
			return ""
		}
	default:
		return ""
	}
}

func MemoryAgentReasoningEffort(agent string) string {
	switch acp.CanonicalAgentName(agent) {
	case acp.AgentCodex, acp.AgentGrok:
		return "low"
	default:
		return ""
	}
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
