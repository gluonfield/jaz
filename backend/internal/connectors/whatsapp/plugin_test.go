package whatsapp

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
	if !slices.Contains(plugin.Capabilities, integrations.CapabilitySync) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		slices.Contains(plugin.Capabilities, integrations.CapabilityMaterialize) ||
		len(plugin.SourceLanes) != 0 {
		t.Fatalf("capabilities = %#v source_lanes = %#v", plugin.Capabilities, plugin.SourceLanes)
	}
	tools := map[string]integrations.PluginTool{}
	for _, tool := range plugin.Tools {
		tools[tool.Name] = tool
	}
	if tools[ToolSearch].Risk != integrations.ActionRiskRead ||
		tools[ToolReadRecent].Risk != integrations.ActionRiskRead ||
		tools[ToolSendMessage].Risk != integrations.ActionRiskWrite {
		t.Fatalf("tools = %#v", tools)
	}
}
