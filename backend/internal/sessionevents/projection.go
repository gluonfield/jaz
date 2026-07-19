package sessionevents

func mergeProjectionEvent(prev, next Event) Event {
	if prev.Type == TypeProviderSubagent && next.Type == TypeProviderSubagent && prev.ProviderSubagent != nil && next.ProviderSubagent != nil {
		subagent := mergeProviderSubagentEvent(*prev.ProviderSubagent, *next.ProviderSubagent)
		next.ProviderSubagent = &subagent
		next.Content = ""
	}
	return next
}

func mergeProviderSubagentEvent(prev, next ProviderSubagentEvent) ProviderSubagentEvent {
	if next.Provider == "" {
		next.Provider = prev.Provider
	}
	if next.ThreadID == "" {
		next.ThreadID = prev.ThreadID
	}
	if next.ParentID == "" {
		next.ParentID = prev.ParentID
	}
	if next.Name == "" {
		next.Name = prev.Name
	}
	if next.Task == "" {
		next.Task = prev.Task
	}
	if next.Role == "" {
		next.Role = prev.Role
	}
	if next.Status == "" {
		next.Status = prev.Status
	}
	if next.Summary == "" {
		next.Summary = prev.Summary
	}
	if next.Prompt == "" {
		next.Prompt = prev.Prompt
	}
	if next.Model == "" {
		next.Model = prev.Model
	}
	if next.ReasoningEffort == "" {
		next.ReasoningEffort = prev.ReasoningEffort
	}
	if next.StartedAtMs == 0 {
		next.StartedAtMs = prev.StartedAtMs
	}
	if next.CompletedAtMs == 0 {
		next.CompletedAtMs = prev.CompletedAtMs
	}
	return next
}

func projectionCoalesceKey(event Event) string {
	if event.Type == "plan_update" && event.Plan != nil {
		return "plan_update:" + event.SessionID
	}
	if event.Type == "proposed_plan" && event.Plan != nil {
		return "proposed_plan:" + event.SessionID
	}
	if event.Type == TypeGoalUpdate && event.Goal != nil {
		return "goal_update:" + event.SessionID
	}
	if event.Type == TypeGoalClear {
		return "goal_update:" + event.SessionID
	}
	if event.ACP != nil && event.ACP.ID != "" && event.ACP.Plan != nil {
		return "acp_plan:" + event.ACP.ID
	}
	if event.Type == "acp" && event.ACP != nil && event.ACP.ID != "" {
		if len(event.ACP.ToolCalls) > 0 {
			return "acp_tools:" + event.ACP.ID
		}
		if event.ACP.Error != "" {
			return "acp_error:" + event.ACP.ID
		}
		return "acp_status:" + event.ACP.ID
	}
	if event.Type == "acp_tool" && event.ACP != nil && event.ACP.ID != "" && len(event.ACP.ToolCalls) > 0 && event.ACP.ToolCalls[0].ID != "" {
		return "acp_tool:" + event.ACP.ID + ":" + event.ACP.ToolCalls[0].ID
	}
	if event.Type == TypeProviderSubagent && event.ProviderSubagent != nil && event.ProviderSubagent.ID != "" {
		return "provider_subagent:" + event.ProviderSubagent.Provider + ":" + event.ProviderSubagent.ID
	}
	if (event.Type == "permission_request" || event.Type == "permission_response") && event.Permission != nil && event.Permission.ID != "" {
		return event.Type + ":" + event.Permission.ID
	}
	return ""
}

func StorageCoalesceKey(event Event) string {
	if event.Type == "acp_tool" && event.ACP != nil && event.ACP.ID != "" && len(event.ACP.ToolCalls) > 0 && event.ACP.ToolCalls[0].ID != "" {
		return "acp_tool:" + event.ACP.ID + ":" + event.ACP.ToolCalls[0].ID
	}
	if event.Type == "acp" && event.ACP != nil && event.ACP.ID != "" && acpStatusKeepsTextStreamOpen(event.ACP) {
		return "acp_status:" + event.ACP.ID
	}
	return ""
}
