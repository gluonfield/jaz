package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
)

type fileMetadata struct {
	Name     string   `json:"name"`
	MIMEType string   `json:"mimeType,omitempty"`
	Parents  []string `json:"parents,omitempty"`
}

func (c APIClient) Create(ctx context.Context, input CreateInput, content io.Reader) (File, error) {
	if input.Title == "" {
		return File{}, fmt.Errorf("title is required")
	}
	contentType := input.ContentMIMEType
	if contentType == "" {
		contentType = input.MIMEType
	}
	if contentType != "" {
		parsed, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return File{}, fmt.Errorf("invalid contentMimeType: %w", err)
		}
		contentType = parsed
	}
	metadata := fileMetadata{Name: input.Title, MIMEType: contentType}
	if input.ParentID != "" {
		metadata.Parents = []string{input.ParentID}
	}
	if content == nil {
		switch contentType {
		case "application/vnd.google-apps.folder", "application/vnd.google-apps.document", "application/vnd.google-apps.spreadsheet", "application/vnd.google-apps.presentation":
		default:
			return File{}, fmt.Errorf("provide filePath, textContent or base64Content, or a Google document/folder contentMimeType")
		}
		var file apiFile
		err := c.json(ctx, http.MethodPost, "/drive/v3/files", fileQuery(), metadata, &file)
		return file.view(true, ""), err
	}
	if contentType == "" {
		return File{}, fmt.Errorf("contentMimeType is required when uploading content")
	}
	if !input.DisableConversionToGoogleType {
		var formats struct {
			Imports map[string][]string `json:"importFormats"`
		}
		if err := c.json(ctx, http.MethodGet, "/drive/v3/about", url.Values{"fields": {"importFormats"}}, nil, &formats); err != nil {
			return File{}, err
		}
		for _, target := range formats.Imports[contentType] {
			if target == "application/vnd.google-apps.document" || target == "application/vnd.google-apps.spreadsheet" || target == "application/vnd.google-apps.presentation" {
				metadata.MIMEType = target
				break
			}
		}
	}
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	part, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}})
	if err != nil {
		return File{}, err
	}
	if err := json.NewEncoder(part).Encode(metadata); err != nil {
		return File{}, err
	}
	if _, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}}); err != nil {
		return File{}, err
	}
	split := envelope.Len()
	if err := writer.Close(); err != nil {
		return File{}, err
	}
	body := io.MultiReader(bytes.NewReader(envelope.Bytes()[:split]), content, bytes.NewReader(envelope.Bytes()[split:]))
	q := fileQuery()
	q.Set("uploadType", "multipart")
	resp, err := c.request(ctx, http.MethodPost, "/upload/drive/v3/files", q, body, "multipart/related; boundary="+writer.Boundary())
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	var file apiFile
	err = json.NewDecoder(resp.Body).Decode(&file)
	return file.view(true, ""), err
}
