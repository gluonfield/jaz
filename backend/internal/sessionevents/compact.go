package sessionevents

import (
	"sort"
	"strings"
	"time"
)

type transcriptItem struct {
	event Event
	index int
}

func CompactTranscript(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	mergedText := compactACPTextRuns(dedupeBySeq(events))
	items := make([]transcriptItem, 0, len(mergedText))
	byKey := map[string]int{}
	for _, item := range mergedText {
		event := item.event
		key := projectionCoalesceKey(event)
		if key != "" {
			if idx, ok := byKey[key]; ok {
				items[idx].event = mergeProjectionEvent(items[idx].event, event)
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
	if len(events) == 0 {
		return nil
	}
	items := compactACPTextRuns(dedupeBySeq(events))
	out := make([]Event, 0, len(items))
	for _, item := range items {
		out = append(out, item.event)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return transcriptItemLess(
			transcriptItem{event: out[i], index: i},
			transcriptItem{event: out[j], index: j},
		)
	})
	return out
}

type compactedTextItem struct {
	event   Event
	index   int
	lastSeq int64
	chunks  []string
}

func compactACPTextRuns(events []Event) []compactedTextItem {
	items := make([]compactedTextItem, 0, len(events))
	openTextIndex := -1
	for sourceIndex, event := range events {
		if isACPTextEvent(event) {
			if openTextIndex >= 0 {
				last := &items[openTextIndex]
				if merged, ok := mergeACPTextEvent(last.event, last.lastSeq, event); ok {
					last.event = merged
					last.chunks = append(last.chunks, acpTextChunk(event))
					if event.Seq != 0 {
						last.lastSeq = event.Seq
					}
					continue
				}
			}
			items = append(items, compactedTextItem{
				event: event, index: sourceIndex, lastSeq: event.Seq, chunks: []string{acpTextChunk(event)},
			})
			openTextIndex = len(items) - 1
			continue
		}
		items = append(items, compactedTextItem{event: event, index: sourceIndex, lastSeq: event.Seq})
		if openTextIndex >= 0 && !keepsACPTextStreamOpen(items[openTextIndex].event, event) {
			openTextIndex = -1
		}
	}
	for i := range items {
		if len(items[i].chunks) < 2 {
			continue
		}
		text := strings.Join(items[i].chunks, "")
		if items[i].event.Type == TypeACPMessage {
			items[i].event.Content = text
		} else {
			acp := *items[i].event.ACP
			acp.Thought = text
			items[i].event.ACP = &acp
		}
	}
	return items
}

func acpTextChunk(event Event) string {
	if event.Type == TypeACPThought {
		return event.ACP.Thought
	}
	return event.Content
}

func mergeACPTextEvent(previous Event, previousSeq int64, next Event) (Event, bool) {
	if !canMergeACPTextEvent(previous, previousSeq, next) {
		return Event{}, false
	}
	merged := previous
	if next.Seq != 0 {
		merged.Seq = next.Seq
	}
	if !next.At.IsZero() {
		merged.At = next.At
	}
	acp := *previous.ACP
	if next.ACP.State != "" {
		acp.State = next.ACP.State
	}
	if next.ACP.StopReason != "" {
		acp.StopReason = next.ACP.StopReason
	}
	if next.ACP.Error != "" {
		acp.Error = next.ACP.Error
	}
	merged.ACP = &acp
	return merged, true
}

func canMergeACPTextEvent(previous Event, previousSeq int64, next Event) bool {
	if !isACPTextEvent(previous) || !isACPTextEvent(next) || previous.Type != next.Type {
		return false
	}
	if previous.SessionID != next.SessionID || previous.ACP.ID != next.ACP.ID {
		return false
	}
	if previous.ACP.TextRunID == "" || previous.ACP.TextRunID != next.ACP.TextRunID {
		return false
	}
	return next.Seq == 0 || previousSeq == 0 || next.Seq > previousSeq
}

func isACPTextEvent(event Event) bool {
	return event.ACP != nil && (event.Type == TypeACPMessage || event.Type == TypeACPThought)
}

func keepsACPTextStreamOpen(text Event, event Event) bool {
	if text.ACP == nil || event.SessionID != text.SessionID {
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
