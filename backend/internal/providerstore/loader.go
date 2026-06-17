package providerstore

import (
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

// Loader adapts the store to provider.CustomProviderLoader so provider.Source can
// overlay DB-backed customs without the provider package importing storage.
type Loader struct {
	Store storage.SettingsStorage
}

func (l Loader) CustomProviderConfigs() (map[string]provider.ModelProviderConfig, error) {
	records, err := List(l.Store)
	if err != nil {
		return nil, err
	}
	out := make(map[string]provider.ModelProviderConfig, len(records))
	for _, record := range records {
		out[record.ID] = record.Config()
	}
	return out, nil
}
