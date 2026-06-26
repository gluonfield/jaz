package acp

import acpschema "github.com/gluonfield/acp-transport/acp"

const (
	claudeModeAuto              = "auto"
	claudeModeBypassPermissions = "bypassPermissions"
)

var baselineModePriority = map[string][]string{
	AgentClaude: {claudeModeBypassPermissions, claudeModeAuto},
	AgentGrok:   {"always-approve"},
}

var defaultBaselineModePriority = []string{"full-access", "yolo", "always-approve"}

// planTurnDefersResult reports whether a plan-requested turn buffers its streamed
// output and publishes a single synthesized proposed_plan at turn end (Codex
// relays `plan` session updates; the native agent uses the update_plan tool).
// Claude is excluded: it surfaces the plan inline via the ExitPlanMode permission
// and streams its plan/implementation text live.
func planTurnDefersResult(planRequested bool, agent string) bool {
	return planRequested && CanonicalAgentName(agent) != AgentClaude
}

func preferredBaselineModeID(agent string, modes []acpschema.SessionMode) string {
	if ids, ok := baselineModePriority[CanonicalAgentName(agent)]; ok {
		return firstSessionMode(modes, ids)
	}
	return firstSessionMode(modes, defaultBaselineModePriority)
}

func baselineModeID(agent string, modes ModeState) string {
	if id := firstModeSnapshot(modes.AvailableModes, baselineModePriority[CanonicalAgentName(agent)]); id != "" {
		return id
	}
	if id := firstModeSnapshot(modes.AvailableModes, defaultBaselineModePriority); id != "" {
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
