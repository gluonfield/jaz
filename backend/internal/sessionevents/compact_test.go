package sessionevents

import (
	"encoding/json"
	"strings"
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

func TestCompactTranscriptKeepsHiddenOwnStatusAsTextBoundary(t *testing.T) {
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "Memory confirms ", compactACPState("thread", "running")),
		compactACP(2, "acp", "", compactACPState("thread", "idle")),
		compactACP(3, "acp", "", compactACPState("thread", "running")),
		compactACP(4, "acp_message", "the ACP surface itself.", compactACPState("thread", "running")),
	})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Seq != 1 || got[0].Content != "Memory confirms " {
		t.Fatalf("first message = %#v", got[0])
	}
	if got[1].Type != "acp" || got[1].Seq != 3 || got[1].ACP.State != "running" {
		t.Fatalf("latest hidden status should remain at seq 3: %#v", got[1])
	}
	if got[2].Seq != 4 || got[2].Content != "the ACP surface itself." {
		t.Fatalf("second message = %#v", got[2])
	}
}

func TestCompactTextChunksKeepsHiddenOwnStatusAsTextBoundary(t *testing.T) {
	events := []Event{
		compactACP(1, "acp_message", "Memory confirms ", compactACPState("thread", "running")),
		compactACP(2, "acp", "", compactACPState("thread", "idle")),
		compactACP(3, "acp", "", compactACPState("thread", "running")),
		compactACP(4, "acp_message", "the ACP surface itself.", compactACPState("thread", "running")),
	}

	got := CompactTextChunks(events)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(got), got)
	}
	if got[0].Seq != 1 || got[0].Content != "Memory confirms " {
		t.Fatalf("first message = %#v", got[0])
	}
	if got[1].Type != "acp" || got[1].Seq != 2 || got[2].Type != "acp" || got[2].Seq != 3 {
		t.Fatalf("hidden statuses should remain separate: %#v", got[1:3])
	}
	if got[3].Seq != 4 || got[3].Content != "the ACP surface itself." {
		t.Fatalf("second message = %#v", got[3])
	}

	runs := CompactTextChunkRuns(events)
	if len(runs) != 0 {
		t.Fatalf("runs = %#v, want none", runs)
	}
}

func TestCompactTextChunksMergesAcrossToolEvents(t *testing.T) {
	pendingTool := compactACPState("thread", "running")
	pendingTool.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "pending"}}
	doneTool := compactACPState("thread", "running")
	doneTool.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "completed"}}

	events := []Event{
		compactACP(1, "acp_message", "Hel", compactACPState("thread", "running")),
		compactACP(2, "acp_message", "lo", compactACPState("thread", "running")),
		compactACP(3, "acp_tool", "", pendingTool),
		compactACP(4, "acp_tool", "", doneTool),
		compactACP(5, "acp_message", "Done.", compactACPState("thread", "running")),
		compactACP(6, "acp_message", "Next", compactACPState("thread", "running")),
		compactACP(8, "acp_message", "gap", compactACPState("thread", "running")),
	}
	got := CompactTextChunks(events)

	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(got), got)
	}
	if got[0].Seq != 3 || got[1].Seq != 4 {
		t.Fatalf("tool events should be preserved separately: %#v", got[:2])
	}
	if got[2].Seq != 6 || got[2].Content != "HelloDone.Next" {
		t.Fatalf("merged text = %#v", got[2])
	}
	if got[3].Seq != 8 || got[3].Content != "gap" {
		t.Fatalf("gap event should not merge: %#v", got[3])
	}
	runs := CompactTextChunkRuns(events)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1: %#v", len(runs), runs)
	}
	if runs[0].Event.Seq != 6 ||
		runs[0].Event.Content != "HelloDone.Next" ||
		len(runs[0].DeleteSeqs) != 3 ||
		runs[0].DeleteSeqs[0] != 1 ||
		runs[0].DeleteSeqs[1] != 2 ||
		runs[0].DeleteSeqs[2] != 5 {
		t.Fatalf("run = %#v", runs[0])
	}
}

func TestCompactTranscriptMergesAcrossToolEvents(t *testing.T) {
	toolACP := compactACPState("thread", "running")
	toolACP.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "pending"}}
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "before", compactACPState("thread", "running")),
		compactACP(2, "acp_tool", "", toolACP),
		compactACP(3, "acp_message", "after", compactACPState("thread", "running")),
		compactACP(5, "acp_message", "gap", compactACPState("thread", "running")),
	})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Seq != 2 || got[0].Type != "acp_tool" {
		t.Fatalf("tool event = %#v", got[0])
	}
	if got[1].Seq != 3 || got[1].Content != "beforeafter" {
		t.Fatalf("merged message = %#v", got[1])
	}
	if got[2].Seq != 5 || got[2].Content != "gap" {
		t.Fatalf("gap message = %#v", got[2])
	}
}

func TestCompactTranscriptDoesNotSplitWordsAcrossToolEvents(t *testing.T) {
	toolACP := compactACPState("thread", "running")
	toolACP.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "completed"}}
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "message chunks, t", compactACPState("thread", "running")),
		compactACP(2, "acp_tool", "", toolACP),
		compactACP(3, "acp_message", "ool calls, and status updates", compactACPState("thread", "running")),
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Type != "acp_tool" {
		t.Fatalf("tool event should be preserved: %#v", got[0])
	}
	if got[1].Content != "message chunks, tool calls, and status updates" {
		t.Fatalf("merged message = %q", got[1].Content)
	}
}

func TestCompactTranscriptKeepsTaskSurfaceAsTextBoundary(t *testing.T) {
	planACP := compactACPState("thread", "running")
	planACP.Plan = []PlanEntry{{Content: "Inspect files", Status: "completed"}}
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "before", compactACPState("thread", "running")),
		compactACP(2, "acp", "", planACP),
		compactACP(3, "acp_message", "after", compactACPState("thread", "running")),
	})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Content != "before" || got[2].Content != "after" {
		t.Fatalf("messages crossed task surface: %#v", got)
	}
}

func TestCompactTranscriptDoesNotMergeAcrossSessions(t *testing.T) {
	other := compactACP(2, "acp_message", "two", compactACPState("thread", "running"))
	other.SessionID = "other"
	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "one", compactACPState("thread", "running")),
		other,
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Content != "one" || got[1].Content != "two" {
		t.Fatalf("messages crossed sessions: %#v", got)
	}
}

func TestCompactTranscriptKeepsOtherSessionSubagentAsTextBoundary(t *testing.T) {
	subagent := Event{
		Seq:       2,
		SessionID: "other",
		Type:      TypeProviderSubagent,
		ProviderSubagent: &ProviderSubagentEvent{
			Provider: "codex",
			ID:       "worker",
			ParentID: "thread",
			Status:   "running",
		},
		At: compactAt(2),
	}

	got := CompactTranscript([]Event{
		compactACP(1, "acp_message", "one", compactACPState("thread", "running")),
		subagent,
		compactACP(2, "acp_message", "two", compactACPState("thread", "running")),
	})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	if got[0].Content != "one" || got[2].Content != "two" {
		t.Fatalf("messages crossed other-session subagent: %#v", got)
	}
}

func TestTextStreamProjectorReplacesPriorTextAcrossToolEvents(t *testing.T) {
	for _, agent := range []string{"codex", "claude_code"} {
		t.Run(agent, func(t *testing.T) {
			textACP := compactACPState("thread", "running")
			textACP.Agent = agent
			toolACP := compactACPState("thread", "running")
			toolACP.Agent = agent
			toolACP.ToolCalls = []ACPToolCall{{ID: "tool-1", Title: "Read file", Status: "completed"}}
			projector := NewTextStreamProjector()

			first := projector.Project(compactACP(1, "acp_message", "message chunks, t", textACP))
			tool := projector.Project(compactACP(2, "acp_tool", "", toolACP))
			second := projector.Project(compactACP(3, "acp_message", "ool calls", textACP))

			if first.Content != "message chunks, t" || len(first.ReplaceSeqs) != 0 {
				t.Fatalf("first projected event = %#v", first)
			}
			if tool.Type != "acp_tool" || len(tool.ReplaceSeqs) != 0 {
				t.Fatalf("tool projected event = %#v", tool)
			}
			if second.Content != "message chunks, tool calls" {
				t.Fatalf("merged content = %q", second.Content)
			}
			if len(second.ReplaceSeqs) != 1 || second.ReplaceSeqs[0] != 1 {
				t.Fatalf("replace seqs = %#v, want [1]", second.ReplaceSeqs)
			}
		})
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

func TestCompactTranscriptCoalescesACPProgressClear(t *testing.T) {
	plan := compactACPState("thread", "running")
	plan.Plan = []ACPPlanEntry{{Content: "first"}}
	clear := compactACPState("thread", "running")
	clear.Plan = []ACPPlanEntry{}

	got := CompactTranscript([]Event{
		compactACP(1, "acp", "", plan),
		compactACP(2, "acp", "", clear),
	})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Seq != 2 || got[0].ACP == nil || got[0].ACP.Plan == nil || len(got[0].ACP.Plan) != 0 {
		t.Fatalf("clear event = %#v", got[0])
	}
}

func TestCompactTranscriptCoalescesProviderSubagent(t *testing.T) {
	got := CompactTranscript([]Event{
		{
			Seq:              1,
			SessionID:        "thread",
			Type:             TypeProviderSubagent,
			ProviderSubagent: &ProviderSubagentEvent{Provider: "codex", ID: "worker-1", Name: "worker", Prompt: "inspect", Status: "running"},
			At:               compactAt(1),
		},
		{
			Seq:              2,
			SessionID:        "thread",
			Type:             TypeProviderSubagent,
			ProviderSubagent: &ProviderSubagentEvent{Provider: "codex", ID: "worker-1", Status: "completed"},
			At:               compactAt(2),
		},
	})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Seq != 2 ||
		got[0].ProviderSubagent == nil ||
		got[0].ProviderSubagent.Status != "completed" ||
		got[0].ProviderSubagent.Name != "worker" ||
		got[0].ProviderSubagent.Prompt != "inspect" {
		t.Fatalf("subagent event = %#v", got[0])
	}
}

func TestACPEventMarshalPreservesExplicitEmptyPlan(t *testing.T) {
	withoutPlan, err := json.Marshal(ACPEvent{ID: "thread", Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutPlan), `"plan"`) {
		t.Fatalf("plan-less event encoded plan: %s", withoutPlan)
	}

	withEmptyPlan, err := json.Marshal(ACPEvent{ID: "thread", Agent: "codex", Plan: []ACPPlanEntry{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withEmptyPlan), `"plan":[]`) {
		t.Fatalf("empty plan was not preserved: %s", withEmptyPlan)
	}
}
