package sessionview

import (
	"strings"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/messagepayload"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type EventResponse struct {
	sessionevents.Event
	Goal *goal.PublicState `json:"goal,omitempty"`
}

func Events(events []sessionevents.Event) []EventResponse {
	out := make([]EventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, Event(event))
	}
	return out
}

func Event(event sessionevents.Event) EventResponse {
	publicGoal := goal.PublicStateFrom(event.Goal)
	event.Goal = nil
	if event.SideChat != nil && len(event.SideChat.Attachments) > 0 {
		sideChat := *event.SideChat
		sideChat.Attachments = attachments(sideChat.Attachments)
		event.SideChat = &sideChat
	}
	return EventResponse{Event: event, Goal: publicGoal}
}

func Messages(records []storage.Message) []storage.Message {
	out := append([]storage.Message(nil), records...)
	for i := range out {
		out[i].Blocks = append([]storage.Block(nil), out[i].Blocks...)
		for j := range out[i].Blocks {
			block := &out[i].Blocks[j]
			if block.Type == storage.BlockTypeAttachment {
				block.ServerPath = ""
				block.URI = displayURI(block.URI)
			}
		}
	}
	return out
}

func attachments(in []messagepayload.Attachment) []messagepayload.Attachment {
	out := append([]messagepayload.Attachment(nil), in...)
	for i := range out {
		out[i].ServerPath = ""
		out[i].URI = displayURI(out[i].URI)
	}
	return out
}

func displayURI(uri string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "file:") {
		return ""
	}
	return uri
}
