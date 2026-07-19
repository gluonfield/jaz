package sessionevents

import (
	"sort"
	"strconv"
	"strings"
)

const (
	ProjectionAppend  = "append"
	ProjectionReplace = "replace"
)

type projectedItem struct {
	event  Event
	index  int
	chunks []string
}

type Projector struct {
	items       []projectedItem
	byKey       map[string]int
	annotator   Annotator
	sourceIndex int
}

type Annotator struct {
	open      Event
	openKey   string
	providers map[string]Event
	index     int
}

func (a Annotator) Clone() Annotator {
	clone := a
	if a.providers != nil {
		clone.providers = make(map[string]Event, len(a.providers))
		for key, event := range a.providers {
			clone.providers[key] = event
		}
	}
	return clone
}

func NewProjector() *Projector {
	return &Projector{byKey: map[string]int{}}
}

func (p *Projector) Apply(event Event) Event {
	index := p.sourceIndex
	p.sourceIndex++
	event = p.annotator.Annotate(event)
	key := event.ProjectionKey
	if key != "" {
		if itemIndex, ok := p.byKey[key]; ok {
			item := &p.items[itemIndex]
			if event.ProjectionOp == ProjectionAppend && isACPTextEvent(event) {
				item.event = p.annotator.open
				item.chunks = append(item.chunks, acpTextChunk(event))
				return event
			}
			item.event = event
			event.ProjectionOp = ProjectionReplace
			return event
		}
		p.byKey[key] = len(p.items)
	}
	item := projectedItem{event: event, index: index}
	if isACPTextEvent(event) {
		item.chunks = []string{acpTextChunk(event)}
	}
	p.items = append(p.items, item)
	return event
}

func (a *Annotator) Annotate(event Event) Event {
	index := a.index
	a.index++
	if isACPTextEvent(event) {
		if a.openKey != "" {
			if merged, ok := mergeACPTextEvent(a.open, a.open.Seq, event); ok {
				a.open = merged
				event.ProjectionKey = a.openKey
				event.ProjectionOp = ProjectionAppend
				return event
			}
		}
		if event.ProjectionKey == "" {
			event.ProjectionKey = textProjectionKey(event, index)
		}
		event.ProjectionOp = ProjectionAppend
		a.open = event
		a.openKey = event.ProjectionKey
		return event
	}
	event = EnsureStatelessProjection(event)
	if event.Type == TypeProviderSubagent && event.ProjectionKey != "" {
		if a.providers == nil {
			a.providers = make(map[string]Event)
		}
		if previous, ok := a.providers[event.ProjectionKey]; ok {
			if previous.ProviderSubagent != nil && event.ProviderSubagent != nil {
				subagent := MergeProviderSubagentEvent(*previous.ProviderSubagent, *event.ProviderSubagent)
				event.ProviderSubagent = &subagent
				event.Content = ""
			}
			event.ProjectionKey = previous.ProjectionKey
			event.ProjectionOp = ProjectionReplace
		}
		if providerSubagentProjectionOpen(event.ProviderSubagent) {
			a.providers[event.ProjectionKey] = event
		} else {
			delete(a.providers, event.ProjectionKey)
		}
	}
	if a.openKey != "" && !keepsACPTextStreamOpen(a.open, event) {
		a.open = Event{}
		a.openKey = ""
	}
	return event
}

func providerSubagentProjectionOpen(subagent *ProviderSubagentEvent) bool {
	if subagent == nil {
		return false
	}
	switch strings.ToLower(subagent.Status) {
	case "completed", "failed", "cancelled", "canceled", "done", "stopped":
		return false
	default:
		return true
	}
}

func EnsureStatelessProjection(event Event) Event {
	if event.ProjectionKey != "" || isACPTextEvent(event) {
		return event
	}
	if key := projectionCoalesceKey(event); key != "" {
		event.ProjectionKey = key
		event.ProjectionOp = ProjectionReplace
	}
	return event
}

func NeedsProjection(event Event) bool {
	return event.ProjectionKey == "" && (isProjectableACPTextEvent(event) || projectionCoalesceKey(event) != "")
}

func (p *Projector) Events() []Event {
	items := make([]transcriptItem, 0, len(p.items))
	for _, item := range p.items {
		event := item.event
		if len(item.chunks) > 1 {
			text := strings.Join(item.chunks, "")
			if event.Type == TypeACPMessage {
				event.Content = text
			} else {
				acp := *event.ACP
				acp.Thought = text
				event.ACP = &acp
			}
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

func textProjectionKey(event Event, sourceIndex int) string {
	if event.ACP == nil || event.ACP.TextRunID == "" {
		return ""
	}
	start := event.Seq
	if start == 0 {
		start = event.At.UnixNano()
	}
	if start == 0 {
		start = int64(sourceIndex + 1)
	}
	return "acp_text:" + event.SessionID + ":" + event.ACP.ID + ":" + event.Type + ":" + event.ACP.TextRunID + ":" + strconv.FormatInt(start, 10) + ":" + strconv.Itoa(sourceIndex+1)
}

func MergeProviderSubagentEvent(prev, next ProviderSubagentEvent) ProviderSubagentEvent {
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
	if next.Task == "" {
		next.Task = prev.Task
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

func CompleteProviderSubagentSnapshot(previous, next Event) Event {
	if previous.Type != TypeProviderSubagent || next.Type != TypeProviderSubagent ||
		previous.ProviderSubagent == nil || next.ProviderSubagent == nil ||
		previous.ProjectionKey == "" || previous.ProjectionKey != next.ProjectionKey {
		return next
	}
	subagent := MergeProviderSubagentEvent(*previous.ProviderSubagent, *next.ProviderSubagent)
	next.ProviderSubagent = &subagent
	next.Content = ""
	return next
}

func NeedsProviderSubagentSnapshot(event Event) bool {
	return ownerScopedProviderSubagentProjection(event)
}

func LatestACPTurn(events []Event, sessionID string) []Event {
	start := 0
	terminals := 0

scan:
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != "acp" || event.ACP == nil || event.ACP.ID != sessionID {
			continue
		}
		switch event.ACP.State {
		case "idle", "failed", "cancelled":
			terminals++
			if terminals == 2 {
				start = i + 1
				break scan
			}
		}
	}
	return CompactTranscript(events[start:])
}

func projectionCoalesceKey(event Event) string {
	if event.Type == "plan_update" && event.Plan != nil {
		return "plan_update:" + event.SessionID
	}
	if event.Type == "proposed_plan" && event.Plan != nil {
		id := event.SessionID
		if event.ACP != nil && event.ACP.ID != "" {
			id = event.ACP.ID
		}
		return "proposed_plan:" + id
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
		if event.ACP.ParentID == event.SessionID && event.ACP.ID != event.SessionID {
			return "acp_status:" + event.ACP.ID
		}
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
		return ProviderSubagentProjectionKey("", *event.ProviderSubagent)
	}
	if event.Type == TypeLoopCreated && event.LoopCreated != nil && event.LoopCreated.LoopID != "" {
		return "loop_created:" + event.LoopCreated.LoopID
	}
	return ""
}

func ProviderSubagentProjectionKey(ownerID string, subagent ProviderSubagentEvent) string {
	if subagent.ID == "" {
		return ""
	}
	key := "provider_subagent:"
	if ownerID != "" {
		key += ownerID + ":"
	}
	return key + subagent.Provider + ":" + subagent.ID
}

func StorageCoalesceKey(event Event) string {
	if NeedsProviderSubagentSnapshot(event) {
		return event.ProjectionKey
	}
	if event.Type == "acp_tool" && event.ACP != nil && event.ACP.ID != "" && len(event.ACP.ToolCalls) > 0 && event.ACP.ToolCalls[0].ID != "" {
		return "acp_tool:" + event.ACP.ID + ":" + event.ACP.ToolCalls[0].ID
	}
	if event.Type == "acp" && event.ACP != nil && event.ACP.ID != "" && acpStatusKeepsTextStreamOpen(event.ACP) {
		return "acp_status:" + event.ACP.ID
	}
	return ""
}

func ownerScopedProviderSubagentProjection(event Event) bool {
	return event.Type == TypeProviderSubagent &&
		event.ProviderSubagent != nil &&
		event.ProjectionKey != "" &&
		event.ProjectionKey != ProviderSubagentProjectionKey("", *event.ProviderSubagent)
}
