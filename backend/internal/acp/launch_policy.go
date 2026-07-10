package acp

import "strings"

func argsForLaunchPolicy(agent string, args []string, mcpServerPolicy string) []string {
	if CanonicalAgentName(agent) != AgentCodex {
		return args
	}
	args = withCodexConfig(args, "features.goals", "false")
	if !restrictedWorkerPolicy(mcpServerPolicy) {
		return args
	}
	args = withoutCodexConfig(args, "features.tool_search_always_defer_mcp_tools")
	for _, key := range []string{"features.browser_use", "features.browser_use_external", "features.in_app_browser"} {
		args = withCodexConfig(args, key, "false")
	}
	return args
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
