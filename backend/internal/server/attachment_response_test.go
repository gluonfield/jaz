package server

import (
	"testing"

	"github.com/wins/jaz/backend/internal/messagepayload"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestMessageRecordsResponseStripsServerLocalAttachmentPaths(t *testing.T) {
	records := []storage.Message{{
		Blocks: []storage.Block{{
			Type:       storage.BlockTypeAttachment,
			ID:         "att_1",
			Name:       "image.png",
			URI:        "file:///server/image.png",
			ServerPath: "/server/image.png",
		}},
	}}

	got := messageRecordsResponse(records)
	block := got[0].Blocks[0]
	if block.ServerPath != "" || block.URI != "" {
		t.Fatalf("attachment block = %#v, want no server-local display resource", block)
	}
	if records[0].Blocks[0].ServerPath == "" || records[0].Blocks[0].URI == "" {
		t.Fatalf("messageRecordsResponse mutated input: %#v", records[0].Blocks[0])
	}
}

func TestSessionEventResponseStripsSideChatServerLocalAttachmentPaths(t *testing.T) {
	event := sessionevents.Event{
		Type: sessionevents.TypeSideChatMessage,
		SideChat: &sessionevents.SideChatEvent{
			ID:      "side_1",
			Role:    "user",
			Content: "look",
			Attachments: []messagepayload.Attachment{{
				ID:         "att_1",
				Name:       "image.png",
				URI:        "file:///server/image.png",
				ServerPath: "/server/image.png",
			}, {
				ID:         "att_2",
				Name:       "remote.png",
				URI:        "https://example.com/remote.png",
				ServerPath: "/server/remote.png",
			}},
		},
	}

	got := sessionEventResponseFrom(event)
	attachments := got.Event.SideChat.Attachments
	if attachments[0].ServerPath != "" || attachments[0].URI != "" {
		t.Fatalf("file attachment = %#v, want no server-local display resource", attachments[0])
	}
	if attachments[1].ServerPath != "" || attachments[1].URI != "https://example.com/remote.png" {
		t.Fatalf("remote attachment = %#v, want remote URI without server path", attachments[1])
	}
	if event.SideChat.Attachments[0].ServerPath == "" || event.SideChat.Attachments[0].URI == "" {
		t.Fatalf("sessionEventResponseFrom mutated input: %#v", event.SideChat.Attachments[0])
	}
}
