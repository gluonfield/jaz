package calendar

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestPluginDescribesGoogleCalendarConnection(t *testing.T) {
	plugin := Plugin()
	if plugin.ID != ProviderID || plugin.Provider.ID != ProviderID || plugin.Name != ProviderName || !plugin.MultiAccount {
		t.Fatalf("plugin = %#v", plugin)
	}
	if plugin.Category != "calendar" || plugin.Icon.Kind != integrations.PluginIconKindAsset || plugin.Icon.Value != ProviderID {
		t.Fatalf("plugin metadata = %#v %#v", plugin.Category, plugin.Icon)
	}
	if !slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityMCP) ||
		slices.Contains(plugin.Capabilities, integrations.CapabilitySync) {
		t.Fatalf("capabilities = %#v", plugin.Capabilities)
	}
	if len(plugin.Auth) != 1 || !slices.Contains(plugin.Auth[0].Scopes, ScopeEvents) || !slices.Contains(plugin.Auth[0].Scopes, ScopeUserInfoEmail) {
		t.Fatalf("auth = %#v", plugin.Auth)
	}
	if len(plugin.Tools) != 2 {
		t.Fatalf("tools = %#v", plugin.Tools)
	}
}

func TestPluginIncludesImplementedGoogleCalendarTools(t *testing.T) {
	tools := map[string]integrations.PluginTool{}
	for _, tool := range Plugin().Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{ToolGetEvents, ToolCreateEvent} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("missing tool %s", name)
		}
		if !slices.Contains(tools[name].RequiredScopes, ScopeEvents) {
			t.Fatalf("%s scopes = %#v", name, tools[name].RequiredScopes)
		}
	}
	if got := tools[ToolGetEvents].Risk; got != integrations.ActionRiskRead {
		t.Fatalf("get events risk = %q", got)
	}
	if got := tools[ToolCreateEvent].Risk; got != integrations.ActionRiskWrite {
		t.Fatalf("create event risk = %q", got)
	}
}
