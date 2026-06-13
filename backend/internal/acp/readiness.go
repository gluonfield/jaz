package acp

import "strings"

type Readiness struct {
	Available bool
	Reason    string
}

func ProbeReadiness(name string, cfg AgentConfig, root string, env map[string]string) Readiness {
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
	probeEnv := NewManager(nil, Config{Root: root, Env: env}, nil).probeEnv(name, cfg)
	auth := ProbeAgentAuth(name, cfg, root, env)
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
	}
	return Readiness{Available: true}
}

func executableAvailable(executable string) error {
	_, err := ResolveExecutable(executable)
	return err
}
