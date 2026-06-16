package acp

import (
	"strings"

	"github.com/wins/jaz/backend/internal/provider"
)

type Readiness struct {
	Available bool
	Reason    string
}

func ProbeReadiness(name string, cfg AgentConfig, root string, env map[string]string) Readiness {
	return ProbeReadinessWithProviders(name, cfg, root, env, nil)
}

func ProbeReadinessWithProviders(name string, cfg AgentConfig, root string, env map[string]string, providers map[string]provider.ModelProviderConfig) Readiness {
	name = CanonicalAgentName(name)
	if strings.TrimSpace(cfg.URL) != "" {
		return Readiness{Available: true}
	}
	command, _ := processCommand(name, cfg)
	if strings.TrimSpace(command) == "" {
		return Readiness{Reason: "command is not configured"}
	}
	if err := executableAvailable(command); err != nil {
		return Readiness{Reason: err.Error()}
	}
	probeEnv := NewManager(nil, Config{Root: root, Env: env, Providers: providers}, nil).probeEnv(name, cfg)
	auth := ProbeAgentAuthWithProviders(name, cfg, root, env, providers)
	switch name {
	case AgentCodex:
		if !auth.Authenticated {
			return Readiness{Reason: auth.Reason}
		}
	case AgentClaude:
		if strings.TrimSpace(probeEnv["CLAUDE_CODE_EXECUTABLE"]) == "" {
			return Readiness{Reason: "Claude Code executable (claude) not found"}
		}
		if !auth.Authenticated {
			return Readiness{Reason: auth.Reason}
		}
	case AgentGrok:
		if !auth.Authenticated {
			return Readiness{Reason: auth.Reason}
		}
	case AgentOpenCode:
		if !auth.Authenticated {
			return Readiness{Reason: auth.Reason}
		}
	}
	return Readiness{Available: true}
}

func executableAvailable(executable string) error {
	_, err := ResolveExecutable(executable)
	return err
}
