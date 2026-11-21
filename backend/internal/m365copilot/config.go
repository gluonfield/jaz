package m365copilot

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	AgentName               = "microsoft_copilot"
	DisplayName             = "Microsoft Copilot Work/School"
	DefaultTenantID         = "organizations"
	PersonalAccountTenantID = "9188040d-6c67-4c5b-b112-36a304b66dad"
	DefaultAuthorityBaseURL = "https://login.microsoftonline.com"
	DefaultGraphBaseURL     = "https://graph.microsoft.com/beta"
	TokenConnectionID       = "acp:microsoft_copilot"
	OAuthCallbackPath       = "/v1/acp/agents/microsoft_copilot/auth/callback"
)

var (
	ErrClientIDRequired = errors.New("set JAZ_MICROSOFT_COPILOT_OAUTH_CLIENT_ID to your Microsoft Entra app registration client ID before signing in")

	requiredScopes = []string{
		"openid",
		"profile",
		"offline_access",
		"Sites.Read.All",
		"Mail.Read",
		"People.Read.All",
		"OnlineMeetingTranscript.Read.All",
		"Chat.Read",
		"ChannelMessage.Read.All",
		"ExternalItem.Read.All",
	}
)

func RequiredScopes() []string {
	return append([]string(nil), requiredScopes...)
}

type OAuthConfig struct {
	ClientID         string
	ClientSecret     string
	TenantID         string
	RedirectURL      string
	UseBroker        bool
	AuthorityBaseURL string
	GraphBaseURL     string
}

func (c OAuthConfig) Normalize() OAuthConfig {
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	c.TenantID = strings.TrimSpace(c.TenantID)
	if c.TenantID == "" {
		c.TenantID = DefaultTenantID
	}
	c.RedirectURL = strings.TrimSpace(c.RedirectURL)
	c.AuthorityBaseURL = strings.TrimRight(strings.TrimSpace(c.AuthorityBaseURL), "/")
	if c.AuthorityBaseURL == "" {
		c.AuthorityBaseURL = DefaultAuthorityBaseURL
	}
	c.GraphBaseURL = strings.TrimRight(strings.TrimSpace(c.GraphBaseURL), "/")
	if c.GraphBaseURL == "" {
		c.GraphBaseURL = DefaultGraphBaseURL
	}
	return c
}

func (c OAuthConfig) ValidateRuntime() error {
	c = c.Normalize()
	if isUnsupportedTenant(c.TenantID) {
		return errors.New("Microsoft 365 Copilot Chat API requires a work or school Microsoft Entra tenant (use \"organizations\" or a tenant ID/domain, not personal accounts)")
	}
	if strings.ContainsAny(c.TenantID, "/?\\ \t\r\n") {
		return fmt.Errorf("invalid Microsoft Entra tenant %q", c.TenantID)
	}
	if err := validateHTTPURL("authority URL", c.AuthorityBaseURL); err != nil {
		return err
	}
	if err := validateHTTPURL("Graph base URL", c.GraphBaseURL); err != nil {
		return err
	}
	if c.RedirectURL != "" {
		if err := validateHTTPURL("OAuth redirect URL", c.RedirectURL); err != nil {
			return err
		}
	}
	return nil
}

func (c OAuthConfig) Validate() error {
	c = c.Normalize()
	if err := c.ValidateRuntime(); err != nil {
		return err
	}
	if c.ClientID == "" {
		return ErrClientIDRequired
	}
	return nil
}

func (c OAuthConfig) AuthURL() string {
	c = c.Normalize()
	return c.AuthorityBaseURL + "/" + url.PathEscape(c.TenantID) + "/oauth2/v2.0/authorize"
}

func (c OAuthConfig) TokenURL() string {
	c = c.Normalize()
	return c.AuthorityBaseURL + "/" + url.PathEscape(c.TenantID) + "/oauth2/v2.0/token"
}

func isUnsupportedTenant(tenant string) bool {
	lower := strings.ToLower(tenant)
	return lower == "consumers" || lower == "common" || lower == PersonalAccountTenantID
}

func validateHTTPURL(label, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("invalid Microsoft Copilot %s %q", label, raw)
	}
	return nil
}
