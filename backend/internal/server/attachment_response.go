package server

import "github.com/wins/jaz/backend/internal/storage"

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
