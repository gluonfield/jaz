package gmail

const (
	// OAuthConnectionID is the legacy single-account connection ID. New Gmail
	// connections use integrations.ConnectionID, but existing rows keep working.
	OAuthConnectionID = "gmail:default"
)

var OAuthScopes = []string{ScopeModify}
