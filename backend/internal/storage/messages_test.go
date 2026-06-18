package storage

import "testing"

func TestAttachmentBlockStoresServerPathWithoutDerivedURI(t *testing.T) {
	block := AttachmentBlock(Attachment{
		ID:         "att_1",
		Name:       "note.txt",
		URI:        "file:///stale.txt",
		ServerPath: "/tmp/note.txt",
	})

	if block.ServerPath != "/tmp/note.txt" || block.URI != "" {
		t.Fatalf("attachment block = %#v, want server path without derived uri", block)
	}
}

func TestAttachmentBlockKeepsLegacyURIWithoutServerPath(t *testing.T) {
	block := AttachmentBlock(Attachment{
		ID:   "att_1",
		Name: "note.txt",
		URI:  "file:///tmp/note.txt",
	})

	if block.URI != "file:///tmp/note.txt" {
		t.Fatalf("attachment block uri = %q", block.URI)
	}
}
