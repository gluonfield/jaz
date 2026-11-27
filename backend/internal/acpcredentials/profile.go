package acpcredentials

import "context"

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
}

type ChatGPTTokens struct {
	AccessToken string `json:"accessToken"`
	AccountID   string `json:"chatgptAccountId"`
}
