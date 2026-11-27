package acpcredentials

import (
	"context"
	"time"
)

const (
	APIKey      = "api_key"
	OAuthBundle = "oauth_bundle"
)

type Resolver func(ctx context.Context, workspaceID, profileID string, rejectedVersion int64) (Profile, error)

type Profile struct {
	ID      string         `json:"profile_id"`
	Agent   string         `json:"agent"`
	Kind    string         `json:"credential_kind"`
	Version int64          `json:"version"`
	APIKey  string         `json:"api_key,omitempty"`
	ChatGPT *ChatGPTTokens `json:"chatgpt,omitempty"`
	OAuth   *OAuthTokens   `json:"oauth,omitempty"`
}

func (p Profile) ValidFor(id string) bool {
	if p.ID != id || p.ID != "acp:"+p.Agent || p.Version <= 0 {
		return false
	}
	switch p.Kind {
	case APIKey:
		return (p.Agent == "claude" || p.Agent == "antigravity" || p.Agent == "grok") && p.APIKey != "" && p.ChatGPT == nil && p.OAuth == nil
	case OAuthBundle:
		if p.APIKey != "" {
			return false
		}
		if p.Agent == "codex" {
			return p.OAuth == nil && p.ChatGPT != nil && p.ChatGPT.AccessToken != "" && p.ChatGPT.AccountID != ""
		}
		return (p.Agent == "claude" || p.Agent == "antigravity") && p.ChatGPT == nil && p.OAuth != nil && p.OAuth.AccessToken != ""
	default:
		return false
	}
}

type ChatGPTTokens struct {
	AccessToken string `json:"accessToken"`
	AccountID   string `json:"chatgptAccountId"`
}

type OAuthTokens struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	AccountID   string    `json:"account_id"`
	Email       string    `json:"email"`
	GCPProject  string    `json:"gcp_project"`
	GCPLocation string    `json:"gcp_location"`
}
