package acp

import acpschema "github.com/gluonfield/acp-transport/acp"

const (
	claudeModeAuto           = "auto"
	claudePlanExitTitle      = "Ready to code?"
	claudePlanExitToolCallID = "exit-plan-mode"
)

var executionModePriority = map[string][]string{
	AgentClaude: {claudeModeAuto},
	AgentGrok:   {"always-approve"},
}

var defaultExecutionModePriority = []string{"full-access", "yolo", "always-approve"}

func executionModeForAgent(agent string, modes []acpschema.SessionMode) string {
	if ids, ok := executionModePriority[CanonicalAgentName(agent)]; ok {
		return firstSessionMode(modes, ids)
	}
	return firstSessionMode(modes, defaultExecutionModePriority)
}

func baselineModeID(agent string, modes ModeState) string {
	if id := firstModeSnapshot(modes.AvailableModes, executionModePriority[CanonicalAgentName(agent)]); id != "" {
		return id
	}
	if id := firstModeSnapshot(modes.AvailableModes, defaultExecutionModePriority); id != "" {
		return id
	}
	if modes.CurrentModeID != "" && modes.CurrentModeID != modes.PlanModeID {
		return modes.CurrentModeID
	}
	for _, mode := range modes.AvailableModes {
		if mode.ID != "" && mode.ID != modes.PlanModeID {
			return mode.ID
		}
	}
	return ""
}

func firstModeSnapshot(modes []ModeSnapshot, ids []string) string {
	for _, id := range ids {
		for _, mode := range modes {
			if mode.ID == id {
				return id
			}
		}
	}
	return ""
}

func planExitPermissionOption(job *Job, req acpschema.RequestPermissionRequest) string {
	if job == nil || CanonicalAgentName(job.ACPAgent) != AgentClaude || !isClaudePlanExitPermission(req) {
		return ""
	}
	job.mu.RLock()
	modes := job.Modes.Clone()
	job.mu.RUnlock()
	target := baselineModeID(job.ACPAgent, modes)
	if target == "" || target == modes.PlanModeID {
		return ""
	}
	return allowedPermissionOption(req.Options, target)
}

func isClaudePlanExitPermission(req acpschema.RequestPermissionRequest) bool {
	if string(req.ToolCall.ToolCallID) == claudePlanExitToolCallID {
		return true
	}
	return req.ToolCall.Kind != nil &&
		*req.ToolCall.Kind == acpschema.ToolKindSwitchMode &&
		req.ToolCall.Title == claudePlanExitTitle
}

func firstSessionMode(modes []acpschema.SessionMode, ids []string) string {
	for _, id := range ids {
		for _, mode := range modes {
			if string(mode.ID) == id {
				return id
			}
		}
	}
	return ""
}

func allowedPermissionOption(options []acpschema.PermissionOption, id string) string {
	for _, option := range options {
		if string(option.OptionID) == id && permissionOptionKindAllows(option.Kind) {
			return id
		}
	}
	return ""
}

func permissionOptionKindAllows(kind acpschema.PermissionOptionKind) bool {
	switch kind {
	case acpschema.PermissionOptionKindAllowAlways, acpschema.PermissionOptionKindAllowOnce:
		return true
	default:
		return false
	}
}
