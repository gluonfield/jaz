package acp

import (
	"context"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/processenv"
)

func prepareCopilotEnv(env map[string]string) {
	processenv.PreserveHost(env, "GH_TOKEN", "GITHUB_TOKEN", "GH_HOST", "GH_CONFIG_DIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "LANG", "LC_ALL", "LC_CTYPE")
	for _, binding := range os.Environ() {
		key, value, _ := strings.Cut(binding, "=")
		if strings.HasPrefix(key, "COPILOT_") && env[key] == "" {
			env[key] = value
		}
	}
}

func resolveCopilotAuth(env map[string]string) resolvedAgentAuth {
	status := resolvedAgentAuth{
		Config: AgentAuthConfig{Mode: AuthModeExistingCLI},
		Source: AuthModeExistingCLI,
	}
	var native struct {
		Authenticated bool `json:"isAuthenticated"`
	}
	if err := copilotCall(context.Background(), env, "auth.getStatus", &native); err != nil {
		status.Reason = err.Error()
		return status
	}
	if native.Authenticated {
		status.markAuthenticated("copilot_auth_status", "")
	} else {
		status.Reason = "Sign in with copilot login --device-code on the Jaz server"
	}
	return status
}
