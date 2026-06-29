package whatsapp

import "encoding/json"

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
		rawMessageText(r.Message),
		rawMessageText(r.MessagePayload),
		rawWebMessageText(r.WebMessage),
		r.QuotedText,
		rawQuotedText(r.Message),
		rawQuotedText(r.MessagePayload),
		rawWebQuotedText(r.WebMessage),
	)
}

type rawWebMessage struct {
	Message json.RawMessage `json:"message"`
}

type rawMessage struct {
	Conversation               string              `json:"conversation"`
	ExtendedTextMessage        *rawTextMessage     `json:"extendedTextMessage"`
	ImageMessage               *rawCaptionMessage  `json:"imageMessage"`
	VideoMessage               *rawCaptionMessage  `json:"videoMessage"`
	DocumentMessage            *rawCaptionMessage  `json:"documentMessage"`
	LocationMessage            *rawLocationMessage `json:"locationMessage"`
	ContactMessage             *rawContactMessage  `json:"contactMessage"`
	ReactionMessage            *rawTextMessage     `json:"reactionMessage"`
	StatusQuotedMessage        *rawTextMessage     `json:"statusQuotedMessage"`
	ViewOnceMessage            *rawWrappedMessage  `json:"viewOnceMessage"`
	ViewOnceMessageV2          *rawWrappedMessage  `json:"viewOnceMessageV2"`
	ViewOnceMessageV2Extension *rawWrappedMessage  `json:"viewOnceMessageV2Extension"`
	EphemeralMessage           *rawWrappedMessage  `json:"ephemeralMessage"`
	DocumentWithCaptionMessage *rawWrappedMessage  `json:"documentWithCaptionMessage"`
	EditedMessage              *rawWrappedMessage  `json:"editedMessage"`
}

type rawTextMessage struct {
	Text        string      `json:"text"`
	ContextInfo *rawContext `json:"contextInfo"`
}

type rawCaptionMessage struct {
	Caption     string      `json:"caption"`
	ContextInfo *rawContext `json:"contextInfo"`
}

type rawLocationMessage struct {
	Name        string      `json:"name"`
	Address     string      `json:"address"`
	ContextInfo *rawContext `json:"contextInfo"`
}

type rawContactMessage struct {
	DisplayName string      `json:"displayName"`
	ContextInfo *rawContext `json:"contextInfo"`
}

type rawWrappedMessage struct {
	Message *rawMessage `json:"message"`
}

type rawContext struct {
	QuotedMessage              *rawMessage                    `json:"quotedMessage"`
	QuestionReplyQuotedMessage *rawQuestionReplyQuotedMessage `json:"questionReplyQuotedMessage"`
}

type rawQuestionReplyQuotedMessage struct {
	QuotedQuestion *rawMessage `json:"quotedQuestion"`
	QuotedResponse *rawMessage `json:"quotedResponse"`
}

func rawMessageText(data json.RawMessage) string {
	message, ok := parseRawMessage(data)
	if !ok {
		return ""
	}
	return message.text()
}

func rawQuotedText(data json.RawMessage) string {
	message, ok := parseRawMessage(data)
	if !ok {
		return ""
	}
	return message.quotedText()
}

func rawWebMessageText(data json.RawMessage) string {
	message, ok := parseRawWebMessage(data)
	if !ok {
		return ""
	}
	return message.text()
}

func rawWebQuotedText(data json.RawMessage) string {
	message, ok := parseRawWebMessage(data)
	if !ok {
		return ""
	}
	return message.quotedText()
}

func parseRawMessage(data json.RawMessage) (rawMessage, bool) {
	if len(data) == 0 {
		return rawMessage{}, false
	}
	var message rawMessage
	if json.Unmarshal(data, &message) != nil {
		return rawMessage{}, false
	}
	return message, true
}

func parseRawWebMessage(data json.RawMessage) (rawMessage, bool) {
	if len(data) == 0 {
		return rawMessage{}, false
	}
	var web rawWebMessage
	if json.Unmarshal(data, &web) != nil {
		return rawMessage{}, false
	}
	return parseRawMessage(web.Message)
}

func (m rawMessage) text() string {
	if text := firstNonEmpty(
		m.Conversation,
		rawText(m.ExtendedTextMessage),
		rawCaption(m.ImageMessage),
		rawCaption(m.VideoMessage),
		rawCaption(m.DocumentMessage),
		rawLocation(m.LocationMessage),
		rawContact(m.ContactMessage),
		rawText(m.ReactionMessage),
		rawText(m.StatusQuotedMessage),
	); text != "" {
		return text
	}
	for _, message := range m.wrappedMessages() {
		if text := message.text(); text != "" {
			return text
		}
	}
	return ""
}

func (m rawMessage) quotedText() string {
	for _, context := range m.contexts() {
		if text := context.quotedText(); text != "" {
			return text
		}
	}
	for _, message := range m.wrappedMessages() {
		if text := message.quotedText(); text != "" {
			return text
		}
	}
	return ""
}

func (m rawMessage) wrappedMessages() []*rawMessage {
	var out []*rawMessage
	for _, wrapped := range []*rawWrappedMessage{
		m.ViewOnceMessage,
		m.ViewOnceMessageV2,
		m.ViewOnceMessageV2Extension,
		m.EphemeralMessage,
		m.DocumentWithCaptionMessage,
		m.EditedMessage,
	} {
		if wrapped != nil && wrapped.Message != nil {
			out = append(out, wrapped.Message)
		}
	}
	return out
}

func (m rawMessage) contexts() []*rawContext {
	var contexts []*rawContext
	for _, context := range []*rawContext{
		rawTextContext(m.ExtendedTextMessage),
		rawCaptionContext(m.ImageMessage),
		rawCaptionContext(m.VideoMessage),
		rawCaptionContext(m.DocumentMessage),
		rawLocationContext(m.LocationMessage),
		rawContactContext(m.ContactMessage),
		rawTextContext(m.ReactionMessage),
		rawTextContext(m.StatusQuotedMessage),
	} {
		if context != nil {
			contexts = append(contexts, context)
		}
	}
	return contexts
}

func (c *rawContext) quotedText() string {
	if c == nil {
		return ""
	}
	if c.QuotedMessage != nil {
		if text := c.QuotedMessage.text(); text != "" {
			return text
		}
	}
	if c.QuestionReplyQuotedMessage != nil {
		return firstNonEmpty(
			rawMessagePointerText(c.QuestionReplyQuotedMessage.QuotedQuestion),
			rawMessagePointerText(c.QuestionReplyQuotedMessage.QuotedResponse),
		)
	}
	return ""
}

func rawText(message *rawTextMessage) string {
	if message == nil {
		return ""
	}
	return message.Text
}

func rawCaption(message *rawCaptionMessage) string {
	if message == nil {
		return ""
	}
	return message.Caption
}

func rawLocation(message *rawLocationMessage) string {
	if message == nil {
		return ""
	}
	return firstNonEmpty(message.Name, message.Address)
}

func rawContact(message *rawContactMessage) string {
	if message == nil {
		return ""
	}
	return message.DisplayName
}

func rawTextContext(message *rawTextMessage) *rawContext {
	if message == nil {
		return nil
	}
	return message.ContextInfo
}

func rawCaptionContext(message *rawCaptionMessage) *rawContext {
	if message == nil {
		return nil
	}
	return message.ContextInfo
}

func rawLocationContext(message *rawLocationMessage) *rawContext {
	if message == nil {
		return nil
	}
	return message.ContextInfo
}

func rawContactContext(message *rawContactMessage) *rawContext {
	if message == nil {
		return nil
	}
	return message.ContextInfo
}

func rawMessagePointerText(message *rawMessage) string {
	if message == nil {
		return ""
	}
	return message.text()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
