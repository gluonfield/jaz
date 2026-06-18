package server

import (
	"strings"

	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
)

type attachmentResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MimeType   string `json:"mime_type,omitempty"`
	Size       int64  `json:"size,omitempty"`
	URI        string `json:"uri"`
	ServerPath string `json:"server_path,omitempty"`
}

func attachmentResponseFromStorage(attachment storage.Attachment) attachmentResponse {
	return attachmentResponse{
		ID:         attachment.ID,
		Name:       attachment.Name,
		MimeType:   attachment.MimeType,
		Size:       attachment.Size,
		URI:        attachmentDisplayURI(attachment.URI, attachment.ServerPath),
		ServerPath: attachment.ServerPath,
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
			if block.Type == storage.BlockTypeAttachment && strings.TrimSpace(block.URI) == "" {
				block.URI = attachmentDisplayURI("", block.ServerPath)
			}
		}
	}
	return out
}

func attachmentDisplayURI(uri, serverPath string) string {
	if uri = strings.TrimSpace(uri); uri != "" {
		return uri
	}
	if serverPath = strings.TrimSpace(serverPath); serverPath != "" {
		return filepathx.FileURI(serverPath)
	}
	return ""
}
