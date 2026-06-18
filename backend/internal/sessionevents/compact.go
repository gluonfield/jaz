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
				items[idx].event = event
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
	for sourceIndex, event := range events {
		if len(items) > 0 {
			last := &items[len(items)-1]
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
	if event.Type == "acp_message" {
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
		if event.Type == "acp_thought" {
			acp.Thought += event.ACP.Thought
		}
		merged.ACP = &acp
	}
	return merged, true
}

func canMergeACPTextEvent(prev Event, prevLastSeq int64, event Event) bool {
	if prev.ACP == nil || event.ACP == nil {
		return false
	}
	if prev.Type != event.Type {
		return false
	}
	if event.Type != "acp_message" && event.Type != "acp_thought" {
		return false
	}
	if prev.ACP.ID != event.ACP.ID {
		return false
	}
	return prevLastSeq == 0 || event.Seq == 0 || event.Seq == prevLastSeq+1
}

func transcriptCoalesceKey(event Event) string {
	if event.Type == "plan_update" && event.Plan != nil {
		return "plan_update:" + event.SessionID
	}
	if event.Type == "proposed_plan" && event.Plan != nil {
		return "proposed_plan:" + event.SessionID
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
