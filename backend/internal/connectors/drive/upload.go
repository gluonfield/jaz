package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
)

type fileMetadata struct {
	Name     string   `json:"name"`
	MIMEType string   `json:"mimeType,omitempty"`
	Parents  []string `json:"parents,omitempty"`
}

func (c APIClient) Create(ctx context.Context, input CreateInput, content io.ReadSeeker) (File, error) {
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
	size, err := content.Seek(0, io.SeekEnd)
	if err != nil {
		return File{}, err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return File{}, err
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return File{}, err
	}
	q := fileQuery()
	q.Set("uploadType", "resumable")
	req, err := c.newRequest(ctx, http.MethodPost, "/upload/drive/v3/files", q, bytes.NewReader(data), "application/json")
	if err != nil {
		return File{}, err
	}
	req.Header.Set("X-Upload-Content-Type", contentType)
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	resp, err := c.do(req)
	if err != nil {
		return File{}, err
	}
	location, err := resp.Location()
	resp.Body.Close()
	if err != nil || location.User != nil || location.Scheme != req.URL.Scheme || location.Host != req.URL.Host {
		return File{}, fmt.Errorf("Google Drive returned an invalid upload location")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, location.String(), content)
	if err != nil {
		return File{}, err
	}
	req.ContentLength = size
	if size == 0 {
		req.Body = http.NoBody
	}
	req.Header.Set("Content-Type", contentType)
	resp, err = c.do(req)
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	var file apiFile
	err = json.NewDecoder(resp.Body).Decode(&file)
	return file.view(true, ""), err
}
