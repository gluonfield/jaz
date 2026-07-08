package server

import (
	"testing"

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
