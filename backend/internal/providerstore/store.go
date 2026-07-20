// Package providerstore persists user-defined model providers in the settings
// store as a single JSON blob (mirroring internal/settings/agents.go). API keys
// are never stored here — they live in the runtime .env keyed by APIKeyEnv.
package providerstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	SettingsNamespace = "providers"
	CustomKey         = "custom"

	APITypeOpenAICompatible = "openai-compatible"
)

// reservedIDs are catalog provider ids a custom provider may never use, so the
// built-ins can't be shadowed or removed.
var reservedIDs = map[string]struct{}{
	provider.ProviderOpenRouter:       {},
	provider.ProviderOpenAI:           {},
	provider.ProviderOllama:           {},
	provider.ProviderModelStudio:      {},
	provider.ProviderQwenCodingPlan:   {},
	provider.ProviderQwenCodingPlanCN: {},
	provider.ProviderQwenTokenPlan:    {},
	provider.ProviderMock:             {},
}

// CustomProvider is a user-defined OpenAI-compatible model provider. The API key
// lives in the runtime .env (keyed by APIKeyEnv), never in this record.
type CustomProvider struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	BaseURL      string    `json:"base_url"`
	APIType      string    `json:"api_type"`
	APIKeyEnv    string    `json:"api_key_env"`
	DefaultModel string    `json:"default_model,omitempty"`
	Icon         string    `json:"icon,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Config maps the record to the runtime provider config the Source overlays.
func (p CustomProvider) Config() provider.ModelProviderConfig {
	p = p.normalized()
	keyEnv := p.APIKeyEnv
	if provider.BaseURLIsLoopback(p.BaseURL) {
		keyEnv = ""
	}
	return provider.ModelProviderConfig{
		Type:         APITypeOpenAICompatible,
		Label:        p.Label,
		BaseURL:      p.BaseURL,
		APIKeyEnv:    keyEnv,
		DefaultModel: p.DefaultModel,
		OpenCode:     true,
		Codex:        true,
	}
}

func (p CustomProvider) normalized() CustomProvider {
	p.APIKeyEnv = apiKeyEnv(p.ID)
	return p
}

// Input is the editable surface of a custom provider — everything but the id,
// timestamps, and the derived APIKeyEnv.
type Input struct {
	Label        string `json:"label"`
	BaseURL      string `json:"base_url"`
	APIType      string `json:"api_type"`
	DefaultModel string `json:"default_model"`
	Icon         string `json:"icon"`
}

type customList struct {
	Providers []CustomProvider `json:"providers"`
}

func List(store storage.SettingsStorage) ([]CustomProvider, error) {
	list, err := load(store)
	if err != nil {
		return nil, err
	}
	return list.Providers, nil
}

func Get(store storage.SettingsStorage, id string) (CustomProvider, bool, error) {
	list, err := load(store)
	if err != nil {
		return CustomProvider{}, false, err
	}
	for _, p := range list.Providers {
		if p.ID == id {
			return p, true, nil
		}
	}
	return CustomProvider{}, false, nil
}

func Create(store storage.SettingsStorage, in Input) (CustomProvider, error) {
	in, err := ValidateInput(in)
	if err != nil {
		return CustomProvider{}, err
	}
	list, err := load(store)
	if err != nil {
		return CustomProvider{}, err
	}
	id := uniqueID(in.Label, list.Providers)
	now := time.Now().UTC()
	record := CustomProvider{
		ID:           id,
		Label:        in.Label,
		BaseURL:      in.BaseURL,
		APIType:      in.APIType,
		APIKeyEnv:    apiKeyEnv(id),
		DefaultModel: in.DefaultModel,
		Icon:         in.Icon,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	list.Providers = append(list.Providers, record)
	if err := save(store, list); err != nil {
		return CustomProvider{}, err
	}
	return record, nil
}

func Update(store storage.SettingsStorage, id string, in Input) (CustomProvider, error) {
	in, err := ValidateInput(in)
	if err != nil {
		return CustomProvider{}, err
	}
	list, err := load(store)
	if err != nil {
		return CustomProvider{}, err
	}
	for i := range list.Providers {
		if list.Providers[i].ID != id {
			continue
		}
		list.Providers[i].Label = in.Label
		list.Providers[i].BaseURL = in.BaseURL
		list.Providers[i].APIType = in.APIType
		list.Providers[i].APIKeyEnv = apiKeyEnv(list.Providers[i].ID)
		list.Providers[i].DefaultModel = in.DefaultModel
		list.Providers[i].Icon = in.Icon
		list.Providers[i].UpdatedAt = time.Now().UTC()
		updated := list.Providers[i]
		if err := save(store, list); err != nil {
			return CustomProvider{}, err
		}
		return updated, nil
	}
	return CustomProvider{}, fmt.Errorf("provider %q not found", id)
}

// Delete removes the record and returns it so the caller can also clear its key.
func Delete(store storage.SettingsStorage, id string) (CustomProvider, error) {
	list, err := load(store)
	if err != nil {
		return CustomProvider{}, err
	}
	for i := range list.Providers {
		if list.Providers[i].ID != id {
			continue
		}
		removed := list.Providers[i]
		list.Providers = append(list.Providers[:i], list.Providers[i+1:]...)
		if err := save(store, list); err != nil {
			return CustomProvider{}, err
		}
		return removed, nil
	}
	return CustomProvider{}, fmt.Errorf("provider %q not found", id)
}

func ValidateInput(in Input) (Input, error) {
	in.Label = strings.TrimSpace(in.Label)
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.APIType = strings.TrimSpace(in.APIType)
	in.DefaultModel = strings.TrimSpace(in.DefaultModel)
	in.Icon = strings.TrimSpace(in.Icon)
	if in.Label == "" {
		return in, errors.New("name is required")
	}
	if in.BaseURL == "" {
		return in, errors.New("endpoint url is required")
	}
	parsed, err := url.Parse(in.BaseURL)
	if err != nil {
		return in, fmt.Errorf("invalid endpoint url: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return in, errors.New("endpoint url must use http or https")
	}
	if parsed.Host == "" {
		return in, errors.New("endpoint url host is required")
	}
	if parsed.User != nil {
		return in, errors.New("endpoint url must not include credentials")
	}
	in.BaseURL = strings.TrimRight(parsed.String(), "/")
	if in.APIType == "" {
		in.APIType = APITypeOpenAICompatible
	}
	if in.APIType != APITypeOpenAICompatible {
		return in, fmt.Errorf("unsupported api type %q; only %q is supported", in.APIType, APITypeOpenAICompatible)
	}
	return in, nil
}

func load(store storage.SettingsStorage) (customList, error) {
	setting, err := store.LoadSetting(SettingsNamespace, CustomKey)
	if err != nil {
		if errors.Is(err, storage.ErrSettingNotFound) {
			return customList{}, nil
		}
		return customList{}, err
	}
	var list customList
	if err := json.Unmarshal(setting.Value, &list); err != nil {
		return customList{}, err
	}
	for i := range list.Providers {
		list.Providers[i] = list.Providers[i].normalized()
	}
	return list, nil
}

func save(store storage.SettingsStorage, list customList) error {
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = store.SaveSetting(SettingsNamespace, CustomKey, data)
	return err
}

func apiKeyEnv(id string) string {
	return provider.ConfiguredAPIKeyEnv(id, provider.ModelProviderConfig{Type: APITypeOpenAICompatible})
}

func uniqueID(label string, existing []CustomProvider) string {
	base := slugify(label)
	if base == "" {
		base = "provider"
	}
	taken := func(id string) bool {
		if _, ok := reservedIDs[id]; ok {
			return true
		}
		if _, ok := provider.ModelProviderByID(id); ok {
			return true
		}
		for _, p := range existing {
			if p.ID == id {
				return true
			}
		}
		return false
	}
	id := base
	for n := 2; taken(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func slugify(label string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
