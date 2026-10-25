package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/wins/jaz/backend/internal/storage"
)

const ProviderOAuth = "openai"
const ProviderAPI = "openai-api-key"
const ProviderClaude = "claude"

type Settings struct {
	Agent    string `json:"agent"`
	Provider string `json:"provider"`
	Voice    string `json:"voice"`
	Model    string `json:"model,omitempty"`
}

type Provider struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type Status struct {
	Settings
	Providers []Provider `json:"providers"`
	Voices    []string   `json:"voices"`
}

func (s *Service) Settings() (Status, error) {
	providers := []Provider{
		{ID: ProviderOAuth, Label: "OpenAI OAuth"},
		{ID: ProviderAPI, Label: "OpenAI API"},
		{ID: ProviderClaude, Label: "Claude", Reason: "Claude voice requires Jaz Desktop."},
	}
	for i := range providers {
		if providers[i].ID == ProviderClaude {
			continue
		}
		_, err := s.credentials(providers[i].ID)
		providers[i].Available = err == nil
		if err != nil {
			providers[i].Reason = err.Error()
		}
	}
	settings := Settings{Agent: "openai", Provider: ProviderOAuth}
	stored, err := s.store.LoadSetting("voice", "live")
	switch {
	case err == nil:
		if err := json.Unmarshal(stored.Value, &settings); err != nil {
			return Status{}, err
		}
	case errors.Is(err, storage.ErrSettingNotFound):
		if !providers[0].Available && providers[1].Available {
			settings.Provider = ProviderAPI
		}
	default:
		return Status{}, err
	}
	settings = settings.withDefaultVoice()
	if err := settings.validate(); err != nil {
		return Status{}, err
	}
	return Status{Settings: settings, Providers: providers, Voices: voices(settings.Provider)}, nil
}

func (s *Service) SaveSettings(settings Settings) (Status, error) {
	settings = settings.withDefaultVoice()
	if err := settings.validate(); err != nil {
		return Status{}, err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return Status{}, err
	}
	if _, err := s.store.SaveSetting("voice", "live", data); err != nil {
		return Status{}, err
	}
	return s.Settings()
}

func (s Settings) validate() error {
	if s.Provider == ProviderClaude {
		if s.Agent != "claude" || len(s.Model) > 128 {
			return fmt.Errorf("%w: choose a Claude voice model", ErrInput)
		}
	} else if s.Agent != "openai" || (s.Provider != ProviderOAuth && s.Provider != ProviderAPI) || s.Model != "" {
		return fmt.Errorf("%w: choose a supported voice provider", ErrInput)
	}
	if !slices.Contains(voices(s.Provider), s.Voice) {
		return fmt.Errorf("%w: choose a voice supported by the selected provider", ErrInput)
	}
	return nil
}

func (s Settings) withDefaultVoice() Settings {
	if s.Voice == "" {
		s.Voice = "marin"
		if s.Provider == ProviderOAuth {
			s.Voice = "cove"
		} else if s.Provider == ProviderClaude {
			s.Voice = "buttery"
		}
	}
	return s
}

func voices(provider string) []string {
	if provider == ProviderClaude {
		return []string{"buttery", "airy", "mellow", "glassy", "rounded"}
	}
	if provider == ProviderOAuth {
		// Codex thread/realtime/listVoices: V3 uses the V1 catalog.
		return []string{"arbor", "breeze", "cove", "ember", "juniper", "maple", "sol", "spruce", "vale"}
	}
	// https://developers.openai.com/api/reference/resources/live/methods/create
	return []string{"alloy", "ash", "ballad", "beacon", "bossa", "cedar", "cinder", "coral", "delta", "echo", "gleam", "marin", "meridian", "quartz", "ripple", "sage", "shimmer", "stone", "tempo", "verse", "vesper", "willow"}
}
