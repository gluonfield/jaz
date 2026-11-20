package mcp

import (
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func AuthorizeOptions(r *http.Request, publicBaseURL string) mcpconfig.AuthorizeOptions {
	options := mcpconfig.AuthorizeOptions{LoopbackAllowed: publicBaseURL == "" && httpapi.TrustedLocalRequest(r)}
	if publicBaseURL == "" {
		publicBaseURL = httpapi.RequestBaseURL(r)
	}
	if publicBaseURL != "" {
		options.RedirectURL = publicBaseURL + mcpconfig.OAuthCallbackPath
	}
	return options
}
