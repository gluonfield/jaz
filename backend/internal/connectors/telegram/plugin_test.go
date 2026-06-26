package telegram

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestPluginAdvertisesImplementedChatCapabilities(t *testing.T) {
	plugin := Plugin()
	if plugin.Icon.Kind != integrations.PluginIconKindAsset || plugin.Icon.Value != ProviderID {
		t.Fatalf("icon = %#v", plugin.Icon)
	}
	if len(plugin.Auth) != 1 || !slices.Contains(plugin.Auth[0].Requires, "JAZ_TELEGRAM_API_ID") || !slices.Contains(plugin.Auth[0].Requires, "JAZ_TELEGRAM_API_HASH") {
		t.Fatalf("auth = %#v", plugin.Auth)
	}
	if !slices.Contains(plugin.Capabilities, integrations.CapabilitySync) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		slices.Contains(plugin.Capabilities, integrations.CapabilityMaterialize) ||
		len(plugin.SourceLanes) != 0 {
		t.Fatalf("capabilities = %#v source_lanes = %#v", plugin.Capabilities, plugin.SourceLanes)
	}
}
