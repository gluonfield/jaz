package whatsapp

import waE2E "go.mau.fi/whatsmeow/proto/waE2E"

func MessageText(message *waE2E.Message) string {
	if message == nil {
		return ""
	}
	if text := firstNonEmpty(
		message.GetConversation(),
		waText(message.GetExtendedTextMessage()),
		waCaption(message.GetImageMessage()),
		waCaption(message.GetVideoMessage()),
		waCaption(message.GetDocumentMessage()),
		waLocation(message.GetLocationMessage()),
		waContact(message.GetContactMessage()),
		waText(message.GetReactionMessage()),
		waText(message.GetStatusQuotedMessage()),
	); text != "" {
		return text
	}
	for _, nested := range messageWrappers(message) {
		if text := MessageText(nested); text != "" {
			return text
		}
	}
	return ""
}

func MessageQuotedText(message *waE2E.Message) string {
	if message == nil {
		return ""
	}
	for _, context := range messageContexts(message) {
		if text := contextQuotedText(context); text != "" {
			return text
		}
	}
	for _, nested := range messageWrappers(message) {
		if text := MessageQuotedText(nested); text != "" {
			return text
		}
	}
	return ""
}

func contextQuotedText(context *waE2E.ContextInfo) string {
	if context == nil {
		return ""
	}
	if text := MessageText(context.GetQuotedMessage()); text != "" {
		return text
	}
	questionReply := context.GetQuestionReplyQuotedMessage()
	if questionReply == nil {
		return ""
	}
	return firstNonEmpty(
		MessageText(questionReply.GetQuotedQuestion()),
		MessageText(questionReply.GetQuotedResponse()),
	)
}

func messageContexts(message *waE2E.Message) []*waE2E.ContextInfo {
	var contexts []*waE2E.ContextInfo
	add := func(context *waE2E.ContextInfo) {
		if context != nil {
			contexts = append(contexts, context)
		}
	}
	if item := message.GetExtendedTextMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetImageMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetVideoMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetDocumentMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetAudioMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetLocationMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetContactMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetStickerMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetLiveLocationMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetContactsArrayMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetButtonsMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetButtonsResponseMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetListMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetListResponseMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetInteractiveMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetInteractiveResponseMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetPollCreationMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetPollResultSnapshotMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetEventMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetOrderMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetProductMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetTemplateMessage(); item != nil {
		add(item.GetContextInfo())
	}
	if item := message.GetTemplateButtonReplyMessage(); item != nil {
		add(item.GetContextInfo())
	}
	return contexts
}

func messageWrappers(message *waE2E.Message) []*waE2E.Message {
	return nonNilMessages(
		wrappedMessage(message.GetViewOnceMessage()),
		wrappedMessage(message.GetViewOnceMessageV2()),
		wrappedMessage(message.GetViewOnceMessageV2Extension()),
		wrappedMessage(message.GetEphemeralMessage()),
		wrappedMessage(message.GetDocumentWithCaptionMessage()),
		wrappedMessage(message.GetEditedMessage()),
		wrappedMessage(message.GetGroupMentionedMessage()),
		wrappedMessage(message.GetBotInvokeMessage()),
	)
}

func wrappedMessage(message *waE2E.FutureProofMessage) *waE2E.Message {
	if message == nil {
		return nil
	}
	return message.GetMessage()
}

func nonNilMessages(messages ...*waE2E.Message) []*waE2E.Message {
	out := make([]*waE2E.Message, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			out = append(out, message)
		}
	}
	return out
}

type textMessage interface {
	GetText() string
}

func waText(message textMessage) string {
	if message == nil {
		return ""
	}
	return message.GetText()
}

type captionMessage interface {
	GetCaption() string
}

func waCaption(message captionMessage) string {
	if message == nil {
		return ""
	}
	return message.GetCaption()
}

func waLocation(message *waE2E.LocationMessage) string {
	if message == nil {
		return ""
	}
	return firstNonEmpty(message.GetName(), message.GetAddress())
}

func waContact(message *waE2E.ContactMessage) string {
	if message == nil {
		return ""
	}
	return message.GetDisplayName()
}
