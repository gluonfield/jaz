package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

type QueuedMessage struct {
	ID            string           `json:"id,omitempty"`
	Text          string           `json:"text"`
	Contexts      []MessageContext `json:"contexts,omitempty"`
	Quotes        []string         `json:"quotes,omitempty"`
	AttachmentIDs []string         `json:"attachment_ids,omitempty"`
	PlanRequested bool             `json:"plan_requested,omitempty"`
	GoalRequested bool             `json:"goal_requested,omitempty"`
}

func NormalizeQueuedMessages(messages []QueuedMessage) []QueuedMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]QueuedMessage, 0, len(messages))
	for _, message := range messages {
		normalized, ok := NormalizeQueuedMessage(message)
		if !ok {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func CanonicalQueuedMessages(messages []QueuedMessage) []QueuedMessage {
	messages = NormalizeQueuedMessages(messages)
	if len(messages) == 0 {
		return nil
	}
	out := append([]QueuedMessage(nil), messages...)
	seen := make(map[string]bool, len(out))
	for i := range out {
		id := strings.TrimSpace(out[i].ID)
		if id == "" || seen[id] {
			id = legacyQueuedMessageID(i, seen)
		}
		out[i].ID = id
		seen[id] = true
	}
	return out
}

func CanonicalSessionQueue(session Session) Session {
	session.QueuedMessages = CanonicalQueuedMessages(session.QueuedMessages)
	if session.PendingSteer != nil {
		pending := CanonicalQueuedMessages([]QueuedMessage{*session.PendingSteer})
		if len(pending) > 0 {
			session.PendingSteer = &pending[0]
		} else {
			session.PendingSteer = nil
		}
	}
	return session
}

func legacyQueuedMessageID(index int, seen map[string]bool) string {
	id := fmt.Sprintf("legacy-%d", index)
	for n := 2; seen[id]; n++ {
		id = fmt.Sprintf("legacy-%d-%d", index, n)
	}
	return id
}

func NormalizeQueuedMessage(message QueuedMessage) (QueuedMessage, bool) {
	message.ID = strings.TrimSpace(message.ID)
	message.Text = strings.TrimSpace(message.Text)
	message.Contexts = NormalizeMessageContexts(append(SelectionContexts(message.Quotes), message.Contexts...))
	message.Quotes = nil
	message.AttachmentIDs = normalizeNonEmpty(message.AttachmentIDs)
	return message, message.Text != ""
}

func NewQueuedMessage(text string, attachmentIDs []string) QueuedMessage {
	return QueuedMessage{Text: strings.TrimSpace(text), AttachmentIDs: normalizeNonEmpty(attachmentIDs)}
}

func normalizeNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MarshalQueuedMessages(messages []QueuedMessage) (string, error) {
	messages = NormalizeQueuedMessages(messages)
	if len(messages) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func MarshalQueuedMessage(message *QueuedMessage) (string, error) {
	if message == nil {
		return "", nil
	}
	normalized, ok := NormalizeQueuedMessage(*message)
	if !ok {
		return "", nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func UnmarshalQueuedMessages(raw string) ([]QueuedMessage, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("queued messages: %w", err)
	}
	messages := make([]QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		var message QueuedMessage
		if err := json.Unmarshal(entry, &message); err == nil {
			messages = append(messages, message)
			continue
		}
		var text string
		if err := json.Unmarshal(entry, &text); err != nil {
			return nil, fmt.Errorf("queued messages: %w", err)
		}
		messages = append(messages, NewQueuedMessage(text, nil))
	}
	return CanonicalQueuedMessages(messages), nil
}

func UnmarshalQueuedMessage(raw string) (*QueuedMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var message QueuedMessage
	if err := json.Unmarshal([]byte(raw), &message); err == nil {
		normalized, ok := NormalizeQueuedMessage(message)
		if !ok {
			return nil, nil
		}
		canonical := CanonicalQueuedMessages([]QueuedMessage{normalized})
		return &canonical[0], nil
	}
	var text string
	if err := json.Unmarshal([]byte(raw), &text); err != nil {
		return nil, fmt.Errorf("queued message: %w", err)
	}
	normalized, ok := NormalizeQueuedMessage(NewQueuedMessage(text, nil))
	if !ok {
		return nil, nil
	}
	canonical := CanonicalQueuedMessages([]QueuedMessage{normalized})
	return &canonical[0], nil
}
