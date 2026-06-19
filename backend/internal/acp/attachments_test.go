package acp

import (
	"path/filepath"
	"strings"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestPromptContentBlocksUsesResourceLinks(t *testing.T) {
	blocks, err := promptContentBlocks("read this", []storage.Attachment{{
		Name:     "note.txt",
		URI:      "file:///tmp/note.txt",
		MimeType: "text/plain",
		Size:     12,
	}}, localAttachmentResources)
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

func TestPromptContentBlocksDerivesResourceURIFromServerPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	blocks, err := promptContentBlocks("read this", []storage.Attachment{{
		Name:       "note.txt",
		URI:        "file:///stale.txt",
		ServerPath: path,
	}}, localAttachmentResources)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := acpschema.DecodeContentBlock(blocks[1])
	if err != nil {
		t.Fatal(err)
	}
	link, ok := decoded.(acpschema.ResourceLinkContentBlock)
	if !ok {
		t.Fatalf("attachment block type = %T, want resource_link", decoded)
	}
	if link.URI != filepathx.FileURI(path) {
		t.Fatalf("resource link uri = %q, want %q", link.URI, filepathx.FileURI(path))
	}
}

func TestPromptContentBlocksRejectsServerLocalAttachmentForRemoteACP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	_, err := promptContentBlocks("read this", []storage.Attachment{{
		Name:       "note.txt",
		ServerPath: path,
	}}, attachmentResourceResolver{})
	if err == nil || !strings.Contains(err.Error(), "remote ACP attachment resources") {
		t.Fatalf("error = %v, want remote ACP attachment resource error", err)
	}
}
