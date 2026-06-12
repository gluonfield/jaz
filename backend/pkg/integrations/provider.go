package integrations

import "strings"

type Provider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Connection struct {
	ID          string   `json:"id"`
	Provider    string   `json:"provider"`
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"account_name,omitempty"`
	Alias       string   `json:"alias,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

func (c Connection) AccountRef() string {
	if alias := NormalizeAlias(c.Alias); alias != "" {
		return alias
	}
	if name := strings.TrimSpace(c.AccountName); name != "" {
		return name
	}
	return strings.TrimSpace(c.AccountID)
}

type OAuthSpec struct {
	AuthURL  string   `json:"auth_url"`
	TokenURL string   `json:"token_url"`
	Scopes   []string `json:"scopes,omitempty"`
}

type ProviderDescriptor interface {
	Provider() Provider
	OAuthSpec() OAuthSpec
}
