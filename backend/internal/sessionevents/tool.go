package sessionevents

import (
	"bytes"
	"encoding/json"
	"slices"
)

// EqualTranscript reports whether two tool calls have the same UI-visible
// state. UpdatedAt and TerminalOutputAt are transport liveness timestamps.
func (a ACPToolCall) EqualTranscript(b ACPToolCall) bool {
	return a.ID == b.ID &&
		a.Title == b.Title &&
		a.Status == b.Status &&
		a.Kind == b.Kind &&
		a.ToolName == b.ToolName &&
		slices.Equal(a.Content, b.Content) &&
		slices.Equal(a.Locations, b.Locations) &&
		equalToolInput(a.RawInput, b.RawInput) &&
		bytes.Equal(a.RawOutput, b.RawOutput) &&
		a.Runtime.equalTranscript(b.Runtime) &&
		a.StartedAt.Equal(b.StartedAt)
}

func (a ACPToolRuntime) equalTranscript(b ACPToolRuntime) bool {
	return a.TerminalID == b.TerminalID &&
		a.TerminalCwd == b.TerminalCwd &&
		a.ParentToolUseID == b.ParentToolUseID &&
		a.ElapsedTimeSeconds == b.ElapsedTimeSeconds &&
		equalOptional(a.TerminalExitCode, b.TerminalExitCode) &&
		equalOptional(a.TerminalExitSignal, b.TerminalExitSignal)
}

func equalToolInput(a, b map[string]any) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	rawA, errA := json.Marshal(a)
	rawB, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(rawA, rawB)
}

func equalOptional[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
