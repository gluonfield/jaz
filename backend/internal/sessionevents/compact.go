package sessionevents

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type transcriptItem struct {
	event Event
	index int
}

type TextChunkCompaction struct {
	Event      Event
	DeleteSeqs []int64
}

func SplitTextEvent(event Event, maxBytes int) []Event {
	if maxBytes <= 0 || !isACPTextEvent(event) {
		return []Event{event}
	}
	text := acpTextChunk(event)
	if len(text) <= maxBytes {
		return []Event{event}
	}
	parts := make([]Event, 0, len(text)/maxBytes+1)
	for len(text) > 0 {
		end := min(maxBytes, len(text))
		for end < len(text) && end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == 0 {
			_, end = utf8.DecodeRuneInString(text)
		}
		part := event
		part.Seq = 0
		if event.Type == TypeACPMessage {
			part.Content = text[:end]
		} else {
			acp := *event.ACP
			acp.Thought = text[:end]
			part.ACP = &acp
		}
		parts = append(parts, part)
		text = text[end:]
	}
	return parts
}

func SplitTextEvents(events []Event, maxBytes int) ([]Event, []int) {
	expanded := make([]Event, 0, len(events))
	last := make([]int, len(events))
	for i, event := range events {
		expanded = append(expanded, SplitTextEvent(event, maxBytes)...)
		last[i] = len(expanded) - 1
	}
	return expanded, last
}

func CompactTranscript(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	projector := NewProjector()
	for _, event := range dedupeBySeq(events) {
		projector.Apply(event)
	}
	return projector.Events()
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

func CompactTextSegments(events []Event, maxBytes int) []TextChunkCompaction {
	var segments []TextChunkCompaction
	var open *compactedTextItem
	openBytes := 0
	flush := func() {
		if open == nil {
			return
		}
		if len(open.chunks) > 1 {
			text := strings.Join(open.chunks, "")
			if open.event.Type == TypeACPMessage {
				open.event.Content = text
			} else {
				acp := *open.event.ACP
				acp.Thought = text
				open.event.ACP = &acp
			}
		}
		segments = append(segments, TextChunkCompaction{Event: open.event, DeleteSeqs: open.deleteSeqs})
		open = nil
		openBytes = 0
	}
	for _, event := range dedupeBySeq(events) {
		if !isACPTextEvent(event) {
			if open != nil && !keepsACPTextStreamOpen(open.event, event) {
				flush()
			}
			continue
		}
		chunk := acpTextChunk(event)
		if open != nil {
			merged, ok := mergeACPTextEvent(open.event, open.lastSeq, event)
			if ok && (maxBytes <= 0 || openBytes+len(chunk) <= maxBytes) {
				if open.event.Seq != 0 && event.Seq != 0 {
					open.deleteSeqs = append(open.deleteSeqs, open.event.Seq)
				}
				open.event = merged
				open.lastSeq = event.Seq
				open.chunks = append(open.chunks, chunk)
				openBytes += len(chunk)
				continue
			}
			flush()
		}
		open = &compactedTextItem{event: event, lastSeq: event.Seq, chunks: []string{chunk}}
		openBytes = len(chunk)
	}
	flush()
	return segments
}

func NeedsStorageCompaction(event Event) bool {
	return isProjectableACPTextEvent(event)
}

type compactedTextItem struct {
	event      Event
	index      int
	lastSeq    int64
	chunks     []string
	deleteSeqs []int64
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

func isProjectableACPTextEvent(event Event) bool {
	return isACPTextEvent(event) && event.ACP.TextRunID != ""
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
