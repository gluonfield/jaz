package sessionevents

import (
	"sort"
	"time"
)

type transcriptItem struct {
	event Event
	index int
}

type TextChunkCompaction struct {
	Event      Event
	DeleteSeqs []int64
}

func CompactTranscript(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	mergedText := compactACPTextRuns(dedupeBySeq(events), false)
	items := make([]transcriptItem, 0, len(mergedText))
	byKey := map[string]int{}
	for _, item := range mergedText {
		event := item.event
		key := transcriptCoalesceKey(event)
		if key != "" {
			if idx, ok := byKey[key]; ok {
				items[idx].event = mergeCoalescedEvent(items[idx].event, event)
				continue
			}
			byKey[key] = len(items)
		}
		items = append(items, transcriptItem{event: event, index: item.index})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return transcriptItemLess(items[i], items[j])
	})
	out := make([]Event, 0, len(items))
	for _, item := range items {
		out = append(out, item.event)
	}
	return out
}

func mergeCoalescedEvent(prev, next Event) Event {
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

func CompactTextChunks(events []Event) []Event {
	compacted, _ := compactTextChunks(events)
	return compacted
}

func CompactTextChunkRuns(events []Event) []TextChunkCompaction {
	_, runs := compactTextChunks(events)
	return runs
}

func compactTextChunks(events []Event) ([]Event, []TextChunkCompaction) {
	if len(events) == 0 {
		return nil, nil
	}
	items := compactACPTextRuns(dedupeBySeq(events), true)
	out := make([]Event, 0, len(items))
	runs := make([]TextChunkCompaction, 0)
	for _, item := range items {
		out = append(out, item.event)
		if item.event.Seq != 0 && len(item.deleteSeqs) > 0 {
			runs = append(runs, TextChunkCompaction{Event: item.event, DeleteSeqs: item.deleteSeqs})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return transcriptItemLess(
			transcriptItem{event: out[i], index: i},
			transcriptItem{event: out[j], index: j},
		)
	})
	sort.SliceStable(runs, func(i, j int) bool {
		return transcriptItemLess(
			transcriptItem{event: runs[i].Event, index: i},
			transcriptItem{event: runs[j].Event, index: j},
		)
	})
	return out, runs
}

type compactedTextItem struct {
	event      Event
	index      int
	lastSeq    int64
	deleteSeqs []int64
}

func compactACPTextRuns(events []Event, trackDeletes bool) []compactedTextItem {
	items := make([]compactedTextItem, 0, len(events))
	openTextIndex := -1
	lastSeqBySession := map[string]int64{}
	for sourceIndex, event := range events {
		if sequenceGap(lastSeqBySession[event.SessionID], event.Seq) {
			openTextIndex = -1
		}
		if event.Seq != 0 {
			lastSeqBySession[event.SessionID] = event.Seq
		}
		if isACPTextEvent(event) {
			if openTextIndex >= 0 {
				last := &items[openTextIndex]
				if merged, ok := mergeACPTextEvent(last.event, last.lastSeq, event); ok {
					if trackDeletes && last.event.Seq != 0 && event.Seq != 0 {
						last.deleteSeqs = append(last.deleteSeqs, last.event.Seq)
					}
					last.event = merged
					if event.Seq != 0 {
						last.lastSeq = event.Seq
					}
					continue
				}
			}
			items = append(items, compactedTextItem{event: event, index: sourceIndex, lastSeq: event.Seq})
			openTextIndex = len(items) - 1
			continue
		}
		items = append(items, compactedTextItem{event: event, index: sourceIndex, lastSeq: event.Seq})
		if openTextIndex >= 0 && !keepsACPTextStreamOpen(items[openTextIndex].event, event) {
			openTextIndex = -1
		}
	}
	return items
}

func dedupeBySeq(events []Event) []Event {
	out := make([]Event, 0, len(events))
	type seqKey struct {
		sessionID string
		seq       int64
	}
	bySeq := map[seqKey]int{}
	for _, event := range events {
		if event.Seq == 0 {
			out = append(out, event)
			continue
		}
		key := seqKey{sessionID: event.SessionID, seq: event.Seq}
		if idx, ok := bySeq[key]; ok {
			out[idx] = event
			continue
		}
		bySeq[key] = len(out)
		out = append(out, event)
	}
	return out
}

func mergeACPTextEvent(prev Event, prevLastSeq int64, event Event) (Event, bool) {
	if !canMergeACPTextEvent(prev, prevLastSeq, event) {
		return Event{}, false
	}
	merged := prev
	if event.Seq != 0 {
		merged.Seq = event.Seq
	}
	if !event.At.IsZero() {
		merged.At = event.At
	}
	if event.Type == TypeACPMessage {
		merged.Content += event.Content
	}
	if prev.ACP != nil {
		acp := *prev.ACP
		if event.ACP.State != "" {
			acp.State = event.ACP.State
		}
		if event.ACP.StopReason != "" {
			acp.StopReason = event.ACP.StopReason
		}
		if event.ACP.Error != "" {
			acp.Error = event.ACP.Error
		}
		if event.Type == TypeACPThought {
			acp.Thought += event.ACP.Thought
		}
		merged.ACP = &acp
	}
	return merged, true
}

func isACPTextEvent(event Event) bool {
	return event.ACP != nil && (event.Type == TypeACPMessage || event.Type == TypeACPThought)
}

func sequenceGap(prevSeq, seq int64) bool {
	return prevSeq != 0 && seq != 0 && seq != prevSeq+1
}

func keepsACPTextStreamOpen(text Event, event Event) bool {
	if text.ACP == nil {
		return false
	}
	if event.SessionID != text.SessionID {
		return false
	}
	if event.ACP != nil && event.ACP.ID == text.ACP.ID {
		if event.Type == "acp_tool" {
			return true
		}
		if event.Type == "acp" {
			return acpStatusKeepsTextStreamOpen(event.ACP)
		}
	}
	return event.Type == TypeProviderSubagent &&
		event.ProviderSubagent != nil &&
		event.ProviderSubagent.ParentID == text.ACP.ID
}

func acpStatusKeepsTextStreamOpen(acp *ACPEvent) bool {
	if acp == nil || acp.Error != "" {
		return false
	}
	if acp.Assistant != "" || acp.Thought != "" || len(acp.Plan) > 0 || len(acp.ToolCalls) > 0 || len(acp.Permissions) > 0 || acp.GoalRequested {
		return false
	}
	switch acp.State {
	case "", "starting", "running":
		return true
	default:
		return false
	}
}

func canMergeACPTextEvent(prev Event, prevLastSeq int64, event Event) bool {
	if prev.ACP == nil || event.ACP == nil {
		return false
	}
	if prev.Type != event.Type {
		return false
	}
	if event.Type != TypeACPMessage && event.Type != TypeACPThought {
		return false
	}
	if prev.SessionID != event.SessionID {
		return false
	}
	if prev.ACP.ID != event.ACP.ID {
		return false
	}
	if event.Seq != 0 && prevLastSeq != 0 && event.Seq <= prevLastSeq {
		return false
	}
	if prev.ACP.TextRunID != "" || event.ACP.TextRunID != "" {
		return prev.ACP.TextRunID != "" && prev.ACP.TextRunID == event.ACP.TextRunID
	}
	return false
}

func transcriptCoalesceKey(event Event) string {
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

func transcriptItemLess(a, b transcriptItem) bool {
	seqA := a.event.Seq
	seqB := b.event.Seq
	if seqA != 0 && seqB != 0 && a.event.SessionID == b.event.SessionID {
		return seqA < seqB
	}
	timeA := eventUnixNano(a.event.At)
	timeB := eventUnixNano(b.event.At)
	if timeA != timeB {
		return timeA < timeB
	}
	if seqA != seqB {
		return seqA < seqB
	}
	return a.index < b.index
}

func eventUnixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
