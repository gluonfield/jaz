package server

import (
	"strings"

	"github.com/wins/jaz/backend/internal/messagepayload"
	"github.com/wins/jaz/backend/internal/storage"
)

type attachmentResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
	URI      string `json:"uri,omitempty"`
}

func attachmentResponseFromStorage(attachment storage.Attachment) attachmentResponse {
	return attachmentResponse{
		ID:       attachment.ID,
		Name:     attachment.Name,
		MimeType: attachment.MimeType,
		Size:     attachment.Size,
		URI:      attachment.URI,
	}
}

func messageRecordsResponse(records []storage.Message) []storage.Message {
	out := make([]storage.Message, len(records))
	copy(out, records)
	for i := range out {
		if len(out[i].Blocks) == 0 {
			continue
		}
		out[i].Blocks = append([]storage.Block(nil), out[i].Blocks...)
		for j := range out[i].Blocks {
			block := &out[i].Blocks[j]
			if block.Type == storage.BlockTypeAttachment {
				block.ServerPath = ""
				block.URI = displayAttachmentURI(block.URI)
			}
		}
	}
	return out
}

func messagePayloadAttachmentsResponse(attachments []messagepayload.Attachment) []messagepayload.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]messagepayload.Attachment, len(attachments))
	copy(out, attachments)
	for i := range out {
		out[i].ServerPath = ""
		out[i].URI = displayAttachmentURI(out[i].URI)
	}
	return out
}

func displayAttachmentURI(uri string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(uri)), "file:") {
		return ""
	}
	return uri
}
