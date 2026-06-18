package acp

import "fmt"

func ResolveAgentSelector(acpAgent, agentName string) (string, error) {
	acpAgent = CanonicalAgentName(acpAgent)
	agentName = CanonicalAgentName(agentName)
	if acpAgent != "" && agentName != "" && acpAgent != agentName {
		return "", fmt.Errorf("acp_agent %q conflicts with agent_name %q", acpAgent, agentName)
	}
	if acpAgent != "" {
		return acpAgent, nil
	}
	return agentName, nil
}
