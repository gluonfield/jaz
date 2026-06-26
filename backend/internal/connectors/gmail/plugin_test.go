package gmail

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestPluginDescribesGmailConnection(t *testing.T) {
	plugin := Plugin()
	if plugin.ID != "gmail" || plugin.Provider.ID != ProviderID || !plugin.MultiAccount {
		t.Fatalf("plugin = %#v", plugin)
	}
	if plugin.Category != "email" || plugin.Icon.Kind != integrations.PluginIconKindAsset || plugin.Icon.Value != "gmail" {
		t.Fatalf("plugin metadata = %#v %#v", plugin.Category, plugin.Icon)
	}
	if plugin.RemoteMCP == nil || plugin.RemoteMCP.URL != RemoteMCPURL || !plugin.RemoteMCP.OAuthSecrets {
		t.Fatalf("remote mcp = %#v", plugin.RemoteMCP)
	}
	if !slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityMCP) ||
		slices.Contains(plugin.Capabilities, integrations.CapabilitySync) ||
		len(plugin.SourceLanes) != 0 {
		t.Fatalf("capabilities = %#v source_lanes = %#v", plugin.Capabilities, plugin.SourceLanes)
	}
	if len(plugin.Auth) == 0 || !slices.Contains(plugin.Auth[0].Scopes, ScopeModify) {
		t.Fatalf("oauth auth = %#v", plugin.Auth)
	}
	for _, unwanted := range []string{ScopeReadonly, ScopeCompose, ScopeSend} {
		if slices.Contains(plugin.Auth[0].Scopes, unwanted) {
			t.Fatalf("oauth scope %q should not be requested in %#v", unwanted, plugin.Auth[0].Scopes)
		}
	}
	if len(plugin.Skills) != 2 || plugin.Skills[0].ID != "gmail" || plugin.Skills[1].ID != "gmail-inbox-triage" {
		t.Fatalf("skills = %#v", plugin.Skills)
	}
}

func TestPluginIncludesImplementedGmailTools(t *testing.T) {
	tools := map[string]integrations.PluginTool{}
	for _, tool := range Plugin().Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		ToolGetProfile,
		ToolSearchThreads,
		ToolReadThread,
		ToolCreateDraft,
		ToolCreateReply,
		ToolSendDraft,
		ToolUpdateDraft,
		ToolListDrafts,
	} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("missing tool %s", name)
		}
	}
	if len(tools) != 8 {
		t.Fatalf("tools = %#v", tools)
	}
	if got := tools[ToolSearchThreads].Risk; got != integrations.ActionRiskRead {
		t.Fatalf("search risk = %q", got)
	}
	for _, name := range []string{ToolCreateDraft, ToolCreateReply, ToolSendDraft, ToolUpdateDraft} {
		if got := tools[name].Risk; got != integrations.ActionRiskWrite {
			t.Fatalf("%s risk = %q", name, got)
		}
	}
	for _, tool := range tools {
		if !slices.Contains(tool.RequiredScopes, ScopeModify) {
			t.Fatalf("%s scopes = %#v", tool.Name, tool.RequiredScopes)
		}
	}
}
