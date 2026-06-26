package whatsapp

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestPluginAdvertisesImplementedChatCapabilities(t *testing.T) {
	plugin := Plugin()
	if !slices.Contains(plugin.Capabilities, integrations.CapabilitySync) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		slices.Contains(plugin.Capabilities, integrations.CapabilityMaterialize) ||
		len(plugin.SourceLanes) != 0 {
		t.Fatalf("capabilities = %#v source_lanes = %#v", plugin.Capabilities, plugin.SourceLanes)
	}
}
