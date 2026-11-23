package cloudsync

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func isCloudUserEvent(kind string) bool {
	switch kind {
	case "user_message", "turn.user", "turn_user", "turn.started", "turn_started":
		return true
	default:
		return false
	}
}

func isCloudAssistantDelta(kind string) bool {
	switch kind {
	case "assistant_delta", "assistant.delta", "agent_message_chunk":
		return true
	default:
		return false
	}
}

func isCloudAssistantMessage(kind string) bool {
	switch kind {
	case "assistant_message", "assistant.message", "turn.assistant", "turn_assistant":
		return true
	default:
		return false
	}
}

func cloudTerminalState(kind string) string {
	switch kind {
	case "turn_completed", "turn.completed":
		return "idle"
	case "turn_failed", "turn.failed":
		return "failed"
	case "turn_interrupted", "turn.interrupted":
		return "cancelled"
	default:
		return ""
	}
}

func payloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func payloadText(p map[string]any) string {
	for _, key := range []string{"text", "prompt", "content", "delta"} {
		if v, ok := p[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func payloadString(p map[string]any, key string) string {
	v, _ := p[key].(string)
	return v
}

func ProjectTranscript(thread storage.CloudThread, rawEvents []storage.CloudThreadEvent) ProjectedTranscript {
	session := thread.ToSession()
	messages := make([]storage.Message, 0)
	events := make([]sessionevents.Event, 0)
	var latestSeq int64
	acpState := "idle"
	if session.Status == storage.StatusRunning {
		acpState = "running"
	}
	var acpError string
	var pendingText strings.Builder
	var pendingSeq int64
	var pendingAt time.Time

	flushPending := func() {
		if pendingText.Len() == 0 {
			return
		}
		text := pendingText.String()
		messages = append(messages, storage.Message{
			ThreadID:  thread.ID,
			Seq:       pendingSeq,
			Role:      "assistant",
			Content:   text,
			Blocks:    []storage.Block{{Type: storage.BlockTypeText, Text: text}},
			CreatedAt: pendingAt,
		})
		pendingText.Reset()
		pendingSeq = 0
		pendingAt = time.Time{}
	}

	for _, ev := range rawEvents {
		if ev.Seq > latestSeq {
			latestSeq = ev.Seq
		}
		p := payloadMap(ev.PayloadJSON)
		text := payloadText(p)

		if isCloudUserEvent(ev.Type) && text != "" {
			flushPending()
			acpError = ""
			messages = append(messages, storage.Message{
				ThreadID:         thread.ID,
				Seq:              ev.Seq,
				Role:             "user",
				FromUser:         true,
				ActorUserID:      ev.ActorUserID,
				ActorDisplayName: payloadString(p, "actor_display_name"),
				ActorEmail:       payloadString(p, "actor_email"),
				Content:          text,
				Blocks:           []storage.Block{{Type: storage.BlockTypeText, Text: text}},
				CreatedAt:        ev.CreatedAt,
			})
			continue
		}

		if isCloudAssistantDelta(ev.Type) {
			if text != "" {
				pendingText.WriteString(text)
				pendingSeq = ev.Seq
				pendingAt = ev.CreatedAt
			}
			continue
		}

		if isCloudAssistantMessage(ev.Type) && text != "" {
			pendingText.Reset()
			messages = append(messages, storage.Message{
				ThreadID:  thread.ID,
				Seq:       ev.Seq,
				Role:      "assistant",
				Content:   text,
				Blocks:    []storage.Block{{Type: storage.BlockTypeText, Text: text}},
				CreatedAt: ev.CreatedAt,
			})
			continue
		}

		if term := cloudTerminalState(ev.Type); term != "" {
			flushPending()
			acpState = term
			acpError = strings.TrimSpace(payloadString(p, "error"))
		}

		events = append(events, cloudEventToSessionEvent(thread.ID, ev, ""))
	}

	var acpAssistant string
	if acpState == "running" && pendingText.Len() > 0 {
		acpAssistant = pendingText.String()
	} else {
		flushPending()
	}

	rev := latestSeq
	if rev < 1 {
		rev = 1
	}
	return ProjectedTranscript{
		Session:         session,
		Messages:        messages,
		Events:          events,
		LatestSeq:       latestSeq,
		HistoryRevision: rev,
		ACPState:        acpState,
		ACPAssistant:    acpAssistant,
		ACPError:        acpError,
	}
}

type StreamProjector struct {
	threadID  string
	assistant strings.Builder
}

func NewStreamProjector(threadID string) *StreamProjector {
	return &StreamProjector{threadID: threadID}
}

func (sp *StreamProjector) Convert(ev storage.CloudThreadEvent) sessionevents.Event {
	p := payloadMap(ev.PayloadJSON)
	switch {
	case isCloudUserEvent(ev.Type):
		sp.assistant.Reset()
	case isCloudAssistantDelta(ev.Type):
		if chunk := payloadText(p); chunk != "" {
			sp.assistant.WriteString(chunk)
		}
		return cloudEventToSessionEvent(sp.threadID, ev, sp.assistant.String())
	case cloudTerminalState(ev.Type) != "":
		sp.assistant.Reset()
	}
	return cloudEventToSessionEvent(sp.threadID, ev, "")
}

func cloudEventToSessionEvent(threadID string, ev storage.CloudThreadEvent, assistantOverride string) sessionevents.Event {
	sessionID := ev.ThreadID
	if sessionID == "" {
		sessionID = threadID
	}
	p := payloadMap(ev.PayloadJSON)
	if ev.Type == "acp" && p != nil && p["acp"] != nil {
		var full sessionevents.Event
		if json.Unmarshal(ev.PayloadJSON, &full) == nil {
			full.Seq = ev.Seq
			full.SessionID = sessionID
			full.Type = "acp"
			full.At = ev.CreatedAt
			return full
		}
	}
	if isCloudAssistantDelta(ev.Type) || isCloudAssistantMessage(ev.Type) {
		text := assistantOverride
		if text == "" {
			text = payloadText(p)
		}
		return sessionevents.Event{
			Seq:       ev.Seq,
			SessionID: sessionID,
			Type:      "acp",
			ACP: &sessionevents.ACPEvent{
				ID:          sessionID,
				Slug:        sessionID,
				Agent:       payloadString(p, "agent"),
				SessionID:   sessionID,
				State:       "running",
				Assistant:   text,
				LastEventAt: ev.CreatedAt,
			},
			At: ev.CreatedAt,
		}
	}
	if term := cloudTerminalState(ev.Type); term != "" {
		return sessionevents.Event{
			Seq:       ev.Seq,
			SessionID: sessionID,
			Type:      "acp",
			ACP: &sessionevents.ACPEvent{
				ID:          sessionID,
				Slug:        sessionID,
				Agent:       payloadString(p, "agent"),
				SessionID:   sessionID,
				State:       term,
				Error:       strings.TrimSpace(payloadString(p, "error")),
				LastEventAt: ev.CreatedAt,
			},
			At: ev.CreatedAt,
		}
	}
	return sessionevents.Event{
		Seq:       ev.Seq,
		SessionID: sessionID,
		Type:      ev.Type,
		Content:   payloadText(p),
		At:        ev.CreatedAt,
	}
}
