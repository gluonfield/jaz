package acp

import (
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const maxProgressTools = 8

// Hydration carries activity metadata, never command arguments or result payloads.
func compactToolProgress(job Job) []sessionevents.ACPToolProgress {
	if job.ToolCalls == nil {
		return append([]sessionevents.ACPToolProgress(nil), job.ToolProgress[:min(len(job.ToolProgress), maxProgressTools+1)]...)
	}
	tools := make([]sessionevents.ACPToolProgress, 0, len(job.ToolCalls))
	for _, call := range job.ToolCalls {
		id := call.ID
		if len(id) > 128 {
			id = id[:128]
		}
		tool := sessionevents.ACPToolProgress{ID: id, StartedAt: call.StartedAt, UpdatedAt: call.UpdatedAt, Program: safeToolProgram(call.RawInput)}
		switch call.Kind {
		case "read", "edit", "delete", "move", "search", "execute", "think", "fetch", "switch_mode", "other":
			tool.Kind = call.Kind
		}
		switch call.Status {
		case "pending", "in_progress", "running", "completed", "failed", "cancelled":
			tool.Status = call.Status
		}
		tools = append(tools, tool)
	}
	sort.SliceStable(tools, func(i, j int) bool { return progressTime(tools[i]).After(progressTime(tools[j])) })
	out := make([]sessionevents.ACPToolProgress, 0, maxProgressTools+1)
	var last *sessionevents.ACPToolProgress
	for _, tool := range tools {
		if tool.Status == "in_progress" || tool.Status == "running" {
			if len(out) < maxProgressTools {
				out = append(out, tool)
			}
		} else if last == nil {
			copy := tool
			last = &copy
		}
	}
	if last != nil {
		out = append(out, *last)
	}
	return out
}

func progressTime(tool sessionevents.ACPToolProgress) time.Time {
	if !tool.UpdatedAt.IsZero() {
		return tool.UpdatedAt
	}
	return tool.StartedAt
}

func safeToolProgram(input map[string]any) string {
	var value any
	for _, key := range []string{"command", "cmd", "script"} {
		if input[key] != nil {
			value = input[key]
			break
		}
	}
	var program string
	switch command := value.(type) {
	case string:
		if fields := strings.Fields(command); len(fields) > 0 {
			program = fields[0]
		}
	case []any:
		if len(command) > 0 {
			program, _ = command[0].(string)
		}
	case []string:
		if len(command) > 0 {
			program = command[0]
		}
	}
	switch program {
	case "ssh":
		return "ssh"
	case "git":
		return "git"
	case "curl":
		return "curl"
	case "python":
		return "python"
	case "python3":
		return "python3"
	case "go":
		return "go"
	case "bun":
		return "bun"
	case "flutter":
		return "flutter"
	case "dart":
		return "dart"
	case "docker":
		return "docker"
	case "npm":
		return "npm"
	case "node":
		return "node"
	case "rg":
		return "rg"
	case "xcodebuild":
		return "xcodebuild"
	}
	return ""
}
