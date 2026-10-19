package drive

import "github.com/wins/jaz/backend/pkg/integrations"

const ProviderID = "google_drive"

var OAuthScopes = []string{
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/drive.file",
}

func Plugin() integrations.Plugin {
	return integrations.Plugin{
		ID:             ProviderID,
		Name:           "Google Drive",
		Description:    "Search, read, and upload files in personal Google Drive and Google Workspace, including shared drives.",
		Examples:       []string{"Find files in my shared folder", "Summarize a document in Google Drive"},
		Provider:       integrations.Provider{ID: ProviderID, Name: "Google Drive"},
		Category:       "productivity",
		Icon:           integrations.PluginIcon{Kind: integrations.PluginIconKindAsset, Value: ProviderID},
		Auth:           []integrations.AuthOption{{Kind: integrations.AuthKindOAuth, Scopes: OAuthScopes}},
		Capabilities:   []integrations.Capability{integrations.CapabilityAct, integrations.CapabilityMCP},
		MultiAccount:   true,
		LocalMCP:       true,
		Implementation: integrations.Implementation{Status: "available", Owner: "jaz"},
	}
}
