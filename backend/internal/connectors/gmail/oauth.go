package gmail

const (
	OAuthClientID = "181926640908-oms6rae8dt4bcdsbvgfqts0b1b9tvhkr.apps.googleusercontent.com"
	// Google desktop OAuth clients cannot keep this confidential; Google documents
	// embedding it in installed-app source, where it is not treated as a secret.
	OAuthClientSecret = "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"
	OAuthAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL     = "https://oauth2.googleapis.com/token"
	OAuthConnectionID = "gmail:default"
)

var OAuthScopes = []string{ScopeModify}
