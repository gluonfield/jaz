package gmail

const (
	OAuthClientID = "jaz-google-oauth-client-id"
	// Google desktop OAuth clients cannot keep this confidential; Google documents
	// embedding it in installed-app source, where it is not treated as a secret.
	OAuthClientSecret = "jaz-google-oauth-client-secret"
	OAuthAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL     = "https://oauth2.googleapis.com/token"
	OAuthConnectionID = "gmail:default"
)

var OAuthScopes = []string{ScopeModify}
