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
	if plugin.RemoteMCP == nil || plugin.RemoteMCP.URL != RemoteMCPURL || !plugin.RemoteMCP.OAuthSecrets {
		t.Fatalf("remote mcp = %#v", plugin.RemoteMCP)
	}
	if !slices.Contains(plugin.Capabilities, integrations.CapabilitySync) ||
		!slices.Contains(plugin.Capabilities, integrations.CapabilityAct) ||
		!slices.Contains(plugin.SourceLanes, "sources/email") {
		t.Fatalf("capabilities = %#v source_lanes = %#v", plugin.Capabilities, plugin.SourceLanes)
	}
	if len(plugin.Skills) != 2 || plugin.Skills[0].ID != "gmail" || plugin.Skills[1].ID != "gmail-inbox-triage" {
		t.Fatalf("skills = %#v", plugin.Skills)
	}
}

func TestPluginIncludesCodexStyleGmailTools(t *testing.T) {
	tools := map[string]integrations.PluginTool{}
	for _, tool := range Plugin().Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"search_emails",
		"read_email_thread",
		"create_draft",
		"send_email",
		"apply_labels_to_emails",
		"bulk_label_matching_emails",
		"archive_emails",
		"delete_emails",
	} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("missing tool %s", name)
		}
	}
	if got := tools["send_email"].RequiredScopes; !slices.Contains(got, ScopeSend) {
		t.Fatalf("send_email scopes = %#v", got)
	}
	if got := tools["bulk_label_matching_emails"].Risk; got != integrations.ActionRiskBulkWrite {
		t.Fatalf("bulk risk = %q", got)
	}
	if got := tools["search_emails"].Risk; got != integrations.ActionRiskRead {
		t.Fatalf("search risk = %q", got)
	}
}
