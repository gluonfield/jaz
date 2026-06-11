package acp

import acpschema "github.com/gluonfield/acp-transport/acp"

const (
	claudeModeAuto              = "auto"
	claudeModeAcceptEdits       = "acceptEdits"
	claudeModeBypassPermissions = "bypassPermissions"
	claudePlanExitTitle         = "Ready to code?"
	claudePlanExitToolCallID    = "exit-plan-mode"
)

var executionModePriority = map[string][]string{
	AgentClaude: {claudeModeBypassPermissions, claudeModeAuto, claudeModeAcceptEdits},
	AgentGrok:   {"always-approve"},
}

var defaultExecutionModePriority = []string{"full-access", "yolo", "always-approve"}

func executionModeForAgent(agent string, modes []acpschema.SessionMode) string {
	if ids, ok := executionModePriority[CanonicalAgentName(agent)]; ok {
		return firstSessionMode(modes, ids)
	}
	return firstSessionMode(modes, defaultExecutionModePriority)
}

func planExitPermissionOptionForAgent(agent string, req acpschema.RequestPermissionRequest) string {
	if CanonicalAgentName(agent) != AgentClaude || !isClaudePlanExitPermission(req) {
		return ""
	}
	return firstAllowedPermissionOption(req.Options, []string{
		claudeModeBypassPermissions,
		claudeModeAuto,
		claudeModeAcceptEdits,
	})
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

func firstAllowedPermissionOption(options []acpschema.PermissionOption, ids []string) string {
	for _, id := range ids {
		for _, option := range options {
			if string(option.OptionID) == id && permissionOptionKindAllows(option.Kind) {
				return id
			}
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
