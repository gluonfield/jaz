package whatsapp

import (
	"encoding/json"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"google.golang.org/protobuf/encoding/protojson"
)

type ContactRecord struct {
	WhatsAppID   string   `json:"whatsapp_id"`
	JID          string   `json:"jid"`
	PhoneNumber  string   `json:"phone_number"`
	Phone        string   `json:"phone"`
	DisplayName  string   `json:"display_name"`
	ContactNames []string `json:"contact_names"`
	FirstName    string   `json:"first_name"`
	FullName     string   `json:"full_name"`
	PushName     string   `json:"push_name"`
	BusinessName string   `json:"business_name"`
}

type MessageRecord struct {
	ID             string          `json:"id,omitempty"`
	Chat           string          `json:"chat,omitempty"`
	Conversation   string          `json:"conversation,omitempty"`
	RemoteJID      string          `json:"remote_jid,omitempty"`
	Sender         string          `json:"sender,omitempty"`
	Participant    string          `json:"participant,omitempty"`
	FromMe         bool            `json:"from_me,omitempty"`
	IsGroup        bool            `json:"is_group,omitempty"`
	PushName       string          `json:"push_name,omitempty"`
	MediaType      string          `json:"media_type,omitempty"`
	Type           string          `json:"type,omitempty"`
	Timestamp      json.RawMessage `json:"timestamp,omitempty"`
	Text           string          `json:"text"`
	QuotedText     string          `json:"quoted_text,omitempty"`
	Message        json.RawMessage `json:"message,omitempty"`
	MessagePayload json.RawMessage `json:"message_payload,omitempty"`
	WebMessage     json.RawMessage `json:"web_message,omitempty"`
}

func (r MessageRecord) ConversationID(fallback string) string {
	return firstNonEmpty(r.Chat, r.Conversation, r.RemoteJID, fallback)
}

func (r MessageRecord) DisplayText() string {
	return firstNonEmpty(
		r.Text,
		messageJSONText(r.Message),
		messageJSONText(r.MessagePayload),
		webMessageJSONText(r.WebMessage),
		r.QuotedText,
		messageJSONQuotedText(r.Message),
		messageJSONQuotedText(r.MessagePayload),
		webMessageJSONQuotedText(r.WebMessage),
	)
}

func messageJSONText(data json.RawMessage) string {
	return MessageText(messageFromJSON(data))
}

func messageJSONQuotedText(data json.RawMessage) string {
	return MessageQuotedText(messageFromJSON(data))
}

func webMessageJSONText(data json.RawMessage) string {
	message := webMessageFromJSON(data)
	if message == nil {
		return ""
	}
	return MessageText(message.GetMessage())
}

func webMessageJSONQuotedText(data json.RawMessage) string {
	message := webMessageFromJSON(data)
	if message == nil {
		return ""
	}
	return MessageQuotedText(message.GetMessage())
}

func messageFromJSON(data json.RawMessage) *waE2E.Message {
	if len(data) == 0 {
		return nil
	}
	var message waE2E.Message
	if protoJSONUnmarshal.Unmarshal(data, &message) != nil {
		return nil
	}
	return &message
}

func webMessageFromJSON(data json.RawMessage) *waWeb.WebMessageInfo {
	if len(data) == 0 {
		return nil
	}
	var message waWeb.WebMessageInfo
	if protoJSONUnmarshal.Unmarshal(data, &message) != nil {
		return nil
	}
	return &message
}

var protoJSONUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
