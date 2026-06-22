package acp

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func acpToolRuntime(meta map[string]any, at time.Time) sessionevents.ACPToolRuntime {
	var out sessionevents.ACPToolRuntime
	if meta == nil {
		return out
	}
	if terminalInfo, ok := meta["terminal_info"].(map[string]any); ok {
		out.TerminalID = metaStringValue(terminalInfo["terminal_id"])
		out.TerminalCwd = metaStringValue(terminalInfo["cwd"])
	}
	if _, ok := meta["terminal_output"]; ok {
		out.TerminalOutputAt = at
	}
	if terminalExit, ok := meta["terminal_exit"].(map[string]any); ok {
		if code, ok := metaIntValue(terminalExit["exit_code"]); ok {
			out.TerminalExitCode = &code
		}
		if signal := metaStringValue(terminalExit["signal"]); signal != "" {
			out.TerminalExitSignal = &signal
		}
	}
	if cc, ok := meta["claudeCode"].(map[string]any); ok {
		out.ParentToolUseID = metaStringValue(cc["parentToolUseId"])
		if response, ok := cc["toolResponse"].(map[string]any); ok {
			out.ElapsedTimeSeconds, _ = metaFloatValue(response["elapsedTimeSeconds"])
		}
	}
	return out
}

func mergeACPToolRuntime(dst *sessionevents.ACPToolRuntime, src sessionevents.ACPToolRuntime) {
	if src.TerminalID != "" {
		dst.TerminalID = src.TerminalID
	}
	if src.TerminalCwd != "" {
		dst.TerminalCwd = src.TerminalCwd
	}
	if src.ParentToolUseID != "" {
		dst.ParentToolUseID = src.ParentToolUseID
	}
	if src.ElapsedTimeSeconds != 0 {
		dst.ElapsedTimeSeconds = src.ElapsedTimeSeconds
	}
	if !src.TerminalOutputAt.IsZero() {
		dst.TerminalOutputAt = src.TerminalOutputAt
	}
	if src.TerminalExitCode != nil {
		code := *src.TerminalExitCode
		dst.TerminalExitCode = &code
	}
	if src.TerminalExitSignal != nil {
		signal := *src.TerminalExitSignal
		dst.TerminalExitSignal = &signal
	}
}

func metaStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func metaIntValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	default:
		return 0, false
	}
}

func metaFloatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := strconv.ParseFloat(v.String(), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
