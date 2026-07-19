package sessionevents

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToolCallTranscriptEqualityIgnoresTransportTimestamps(t *testing.T) {
	now := time.Now().UTC()
	a := ACPToolCall{
		ID:        "tool-1",
		Title:     "Run tests",
		Status:    "running",
		Content:   []ACPToolContent{{Type: "text", Text: "working"}},
		RawInput:  map[string]any{"package": "./..."},
		RawOutput: json.RawMessage(`{"state":"running"}`),
		Runtime:   ACPToolRuntime{TerminalID: "term-1", TerminalOutputAt: now},
		StartedAt: now,
		UpdatedAt: now,
	}
	b := a
	b.UpdatedAt = now.Add(time.Second)
	b.Runtime.TerminalOutputAt = now.Add(time.Second)
	if !a.EqualTranscript(b) {
		t.Fatal("transport timestamps changed transcript equality")
	}
	b.Status = "completed"
	if a.EqualTranscript(b) {
		t.Fatal("semantic status change was ignored")
	}
}
