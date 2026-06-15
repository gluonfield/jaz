package sessionevents

import (
	"testing"
	"time"
)

var compactBaseTime = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

func compactAt(seq int64) time.Time {
	return compactBaseTime.Add(time.Duration(seq) * time.Second)
}

func compactACP(seq int64, eventType, content string, acp *ACPEvent) Event {
	return Event{
		Seq:       seq,
		SessionID: "thread",
		Type:      eventType,
		Content:   content,
		ACP:       acp,
		At:        compactAt(seq),
	}
}

func compactACPState(id, state string) *ACPEvent {
	return &ACPEvent{
		ID:        id,
		Slug:      id,
		Agent:     "codex",
		SessionID: "acp-" + id,
		State:     state,
	}
}

func TestCompactTranscriptMergesAdjacentACPText(t *testing.T) {
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "Hel", compactACPState("thread", "running")),
		compactACP(2, "acp_message", "lo", compactACPState("thread", "running")),
		compactACP(3, "acp_thought", "", func() *ACPEvent {
			acp := compactACPState("thread", "running")
			acp.Thought = "Rea"
			return acp
		}()),
		compactACP(4, "acp_thought", "", func() *ACPEvent {
			acp := compactACPState("thread", "running")
			acp.Thought = "son"
			return acp
		}()),
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Seq != 2 || got[0].Content != "Hello" {
		t.Fatalf("merged message = seq %d content %q", got[0].Seq, got[0].Content)
	}
	if got[1].Seq != 4 || got[1].ACP == nil || got[1].ACP.Thought != "Reason" {
		t.Fatalf("merged thought = %#v", got[1])
	}
}

func TestCompactTranscriptKeepsVisibleBoundaries(t *testing.T) {
	toolACP := compactACPState("thread", "running")
	toolACP.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "pending"}}
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "before", compactACPState("thread", "running")),
		compactACP(2, "acp_tool", "", toolACP),
		compactACP(3, "acp_message", "after", compactACPState("thread", "running")),
		compactACP(5, "acp_message", "gap", compactACPState("thread", "running")),
	})

	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(got), got)
	}
	if got[0].Content != "before" || got[2].Content != "after" || got[3].Content != "gap" {
		t.Fatalf("messages crossed a boundary: %#v", got)
	}
}

func TestCompactTranscriptCoalescesSemanticUpdates(t *testing.T) {
	pendingTool := compactACPState("thread", "running")
	pendingTool.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "pending"}}
	doneTool := compactACPState("thread", "running")
	doneTool.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "completed"}}
	planA := compactACPState("thread", "running")
	planA.Plan = []ACPPlanEntry{{Content: "first"}}
	planB := compactACPState("thread", "running")
	planB.Plan = []ACPPlanEntry{{Content: "second"}}

	got := CompactTranscript([]Event{
		compactACP(1, "acp", "", compactACPState("thread", "running")),
		compactACP(2, "acp", "", compactACPState("thread", "idle")),
		compactACP(3, "acp_tool", "", pendingTool),
		compactACP(4, "acp_tool", "", doneTool),
		compactACP(5, "acp", "", planA),
		compactACP(6, "acp", "", planB),
		{
			Seq:        7,
			SessionID:  "thread",
			Type:       "permission_request",
			Permission: &ACPPermission{ID: "perm-1", Title: "Approve"},
			At:         compactAt(7),
		},
		{
			Seq:        8,
			SessionID:  "thread",
			Type:       "permission_response",
			Permission: &ACPPermission{ID: "perm-1", Status: "selected"},
			At:         compactAt(8),
		},
	})

	if len(got) != 5 {
		t.Fatalf("len = %d, want 5: %#v", len(got), got)
	}
	if got[0].Seq != 2 || got[0].ACP.State != "idle" {
		t.Fatalf("status event = %#v", got[0])
	}
	if got[1].Seq != 4 || got[1].ACP.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool event = %#v", got[1])
	}
	if got[2].Seq != 6 || got[2].ACP.Plan[0].Content != "second" {
		t.Fatalf("plan event = %#v", got[2])
	}
	if got[3].Type != "permission_request" || got[4].Type != "permission_response" {
		t.Fatalf("permission events = %#v", got[3:])
	}
}
