package acp

import (
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestPromptContentBlocksUsesResourceLinks(t *testing.T) {
	blocks, err := promptContentBlocks("read this", []storage.Attachment{{
		Name:     "note.txt",
		URI:      "file:///tmp/note.txt",
		MimeType: "text/plain",
		Size:     12,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want text + resource_link", len(blocks))
	}

	first, err := acpschema.DecodeContentBlock(blocks[0])
	if err != nil {
		t.Fatal(err)
	}
	text, ok := first.(acpschema.TextContentBlock)
	if !ok || text.Text != "read this" {
		t.Fatalf("text block = %#v", first)
	}

	second, err := acpschema.DecodeContentBlock(blocks[1])
	if err != nil {
		t.Fatal(err)
	}
	link, ok := second.(acpschema.ResourceLinkContentBlock)
	if !ok {
		t.Fatalf("attachment block type = %T, want resource_link", second)
	}
	if link.Name != "note.txt" || link.URI != "file:///tmp/note.txt" || link.MimeType != "text/plain" || link.Size != 12 {
		t.Fatalf("resource link = %#v", link)
	}
}
