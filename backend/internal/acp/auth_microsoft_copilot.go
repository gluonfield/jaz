package acp

import (
	"context"
	"errors"
)

const microsoftCopilotLoginLabel = "Microsoft Work/School OAuth"

var errMicrosoftCopilotServiceNotConfigured = errors.New("Microsoft Copilot OAuth service is not configured")

func resolveMicrosoftCopilotAuth(cfg AgentConfig) resolvedAgentAuth {
	status := resolvedAgentAuth{
		Config: AgentAuthConfig{Mode: AuthModeJazProfile},
		Source: AuthModeJazProfile,
	}
	if cfg.MicrosoftCopilot == nil {
		status.Reason = errMicrosoftCopilotServiceNotConfigured.Error()
		return status
	}
	st := cfg.MicrosoftCopilot.Status(context.Background())
	if st.Authenticated {
		status.markAuthenticated("microsoft_oauth", AuthKindOAuth)
		return status
	}
	status.Reason = st.Reason
	return status
}

func microsoftCopilotLoginCommand(cfg AgentConfig) AgentAuthStatus {
	if cfg.MicrosoftCopilot == nil {
		return AgentAuthStatus{
			LoginCommandReason: errMicrosoftCopilotServiceNotConfigured.Error(),
		}
	}
	if err := cfg.MicrosoftCopilot.ValidateConfig(); err != nil {
		return AgentAuthStatus{
			LoginCommandReason: err.Error(),
		}
	}
	return AgentAuthStatus{
		LoginCommand:          microsoftCopilotLoginLabel,
		LoginCommandAvailable: true,
	}
}

func disconnectMicrosoftCopilotAuth(ctx context.Context, cfg AgentConfig) error {
	if cfg.MicrosoftCopilot == nil {
		return errMicrosoftCopilotServiceNotConfigured
	}
	return cfg.MicrosoftCopilot.Disconnect(ctx)
}
