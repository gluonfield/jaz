package server

import "github.com/wins/jaz/backend/internal/acp"

type acpAuthStatusResponse struct {
	Authenticated         bool                `json:"authenticated"`
	Ready                 bool                `json:"ready"`
	Reason                string              `json:"reason,omitempty"`
	StoragePath           string              `json:"storage_path,omitempty"`
	AuthMode              string              `json:"auth_mode,omitempty"`
	AuthPath              string              `json:"auth_path,omitempty"`
	AuthSource            string              `json:"auth_source,omitempty"`
	AuthEvidence          string              `json:"auth_evidence,omitempty"`
	AuthKind              string              `json:"auth_kind,omitempty"`
	RecommendedAuth       acp.AgentAuthConfig `json:"recommended_auth,omitempty"`
	APIKey                acp.AgentAPIKeySpec `json:"api_key,omitempty"`
	APIKeyConfigured      bool                `json:"api_key_configured"`
	LoginCommand          string              `json:"login_command,omitempty"`
	LoginCommandAvailable bool                `json:"login_command_available"`
	LoginCommandReason    string              `json:"login_command_reason,omitempty"`
	RefreshOwner          string              `json:"refresh_owner,omitempty"`
}

func newACPAuthStatusResponse(auth acp.AgentAuthStatus, readiness acp.Readiness) acpAuthStatusResponse {
	return acpAuthStatusResponse{
		Authenticated:         auth.Authenticated,
		Ready:                 readiness.Available,
		Reason:                firstMessage(readiness.Reason, auth.Reason),
		StoragePath:           auth.StoragePath,
		AuthMode:              auth.AuthMode,
		AuthPath:              auth.AuthPath,
		AuthSource:            auth.AuthSource,
		AuthEvidence:          auth.AuthEvidence,
		AuthKind:              auth.AuthKind,
		RecommendedAuth:       auth.RecommendedAuth,
		APIKey:                auth.APIKey,
		APIKeyConfigured:      auth.APIKeyConfigured,
		LoginCommand:          auth.LoginCommand,
		LoginCommandAvailable: auth.LoginCommandAvailable,
		LoginCommandReason:    auth.LoginCommandReason,
		RefreshOwner:          auth.RefreshOwner,
	}
}
