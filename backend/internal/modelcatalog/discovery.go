package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/providerstore"
	"github.com/wins/jaz/backend/internal/storage"
)

var ErrDiscoveryInput = errors.New("invalid catalog request")
var ErrDiscoveryProvider = errors.New("catalog provider not found")

type DiscoveryInput struct {
	ProviderID, BaseURL, ModelsURL, APIKey string
	Headers                                map[string]string
	Save                                   bool
}

type DiscoveryResult struct {
	Models     []string
	CatalogURL string
	Saved      bool
}

type Discovery struct {
	Store     storage.SettingsStorage
	LookupKey func(string) string
	Reload    func() error
}

func (s Discovery) Fetch(ctx context.Context, input DiscoveryInput) (DiscoveryResult, error) {
	id := strings.TrimSpace(input.ProviderID)
	base, endpoint, key := strings.TrimSpace(input.BaseURL), strings.TrimSpace(input.ModelsURL), strings.TrimSpace(input.APIKey)
	headers := input.Headers
	var record providerstore.CustomProvider
	if id != "" {
		if s.Store == nil {
			return DiscoveryResult{}, errors.New("settings store is not configured")
		}
		found, ok, err := providerstore.Get(s.Store, id)
		if err != nil {
			return DiscoveryResult{}, err
		}
		if !ok {
			return DiscoveryResult{}, fmt.Errorf("%w: %s", ErrDiscoveryProvider, id)
		}
		record = found
		if key == "" && ((base != "" && base != record.BaseURL) || (endpoint != "" && endpoint != record.ModelsURL) || (headers != nil && !maps.Equal(headers, record.Headers))) {
			return DiscoveryResult{}, fmt.Errorf("%w: changed catalog settings require an explicit API key or saving the provider first", ErrDiscoveryInput)
		}
		if base == "" {
			base = record.BaseURL
		}
		if endpoint == "" {
			endpoint = record.ModelsURL
		}
		if headers == nil {
			headers = record.Headers
		}
		if key == "" && s.LookupKey != nil {
			key = s.LookupKey(record.APIKeyEnv)
		}
	}
	if input.Save && id == "" {
		return DiscoveryResult{}, fmt.Errorf("%w: saving requires a provider ID", ErrDiscoveryInput)
	}
	if base == "" && endpoint == "" {
		return DiscoveryResult{}, fmt.Errorf("%w: base_url or models_url is required", ErrDiscoveryInput)
	}
	var endpoints []string
	if endpoint != "" {
		endpoints = append(endpoints, endpoint)
	}
	if url, err := provider.ModelsURL(base); err == nil && url != endpoint {
		endpoints = append(endpoints, url)
	}
	if trimmed := strings.TrimSuffix(base, "/"); strings.HasSuffix(trimmed, "/openai/v1") {
		endpoints = append(endpoints, strings.TrimSuffix(trimmed, "/openai/v1")+"/v1/models")
	}
	var result DiscoveryResult
	for _, url := range endpoints {
		models, err := FetchModels(ctx, url, key, headers)
		if err != nil || len(models) == 0 {
			continue
		}
		result.CatalogURL = url
		for _, model := range models {
			result.Models = append(result.Models, model.Value)
		}
		break
	}
	if len(result.Models) == 0 {
		return result, fmt.Errorf("%w: no models returned from configured catalog endpoints", ErrCatalogUnavailable)
	}
	if input.Save {
		_, err := providerstore.Update(s.Store, id, providerstore.Input{
			Label: record.Label, BaseURL: record.BaseURL, DefaultModel: record.DefaultModel,
			Capabilities: record.Capabilities, Headers: record.Headers, Icon: record.Icon,
			Models: result.Models, ModelsURL: result.CatalogURL,
		})
		if err != nil {
			return result, err
		}
		if s.Reload != nil {
			if err := s.Reload(); err != nil {
				return result, err
			}
		}
		result.Saved = true
	}
	return result, nil
}
