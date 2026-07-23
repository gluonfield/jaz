package acp

import "strings"

func argsForLaunchPolicy(agent string, args []string, _ string) []string {
	if CanonicalAgentName(agent) != AgentCodex {
		return args
	}
	return withCodexConfig(args, "features.goals", "false")
}

func withoutCodexConfig(args []string, key string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--" {
			return append(out, args[i:]...)
		}
		if arg == "-c" && i+1 < len(args) && codexConfigArgKey(args[i+1]) == key {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-c=") && codexConfigArgKey(strings.TrimPrefix(arg, "-c=")) == key {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func codexConfigArgKey(arg string) string {
	arg = strings.TrimSpace(arg)
	key, _, _ := strings.Cut(arg, "=")
	return strings.TrimSpace(key)
}

func withCodexConfig(args []string, key, value string) []string {
	args = withoutCodexConfig(args, key)
	return insertBeforeArg(args, "--", "-c", key+"="+value)
}
