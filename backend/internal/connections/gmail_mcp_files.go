package connections

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/filepathx"
)

func gmailFileAttachments(paths []string) ([]gmailconnector.AttachmentContent, error) {
	var attachments []gmailconnector.AttachmentContent
	remaining := int64(gmailconnector.MaxAttachmentBytes)
	for _, raw := range paths {
		path, err := filepathx.FromUserInput(raw)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("attachment path must be absolute and accessible to the Jaz backend: %q", path)
		}
		attachment, err := readGmailFile(path, remaining)
		if err != nil {
			return nil, fmt.Errorf("attachment %q: %w", path, err)
		}
		remaining -= int64(len(attachment.Data))
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func readGmailFile(path string, remaining int64) (gmailconnector.AttachmentContent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return gmailconnector.AttachmentContent{}, err
	}
	if !info.Mode().IsRegular() {
		return gmailconnector.AttachmentContent{}, fmt.Errorf("path must identify a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return gmailconnector.AttachmentContent{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return gmailconnector.AttachmentContent{}, err
	}
	if !info.Mode().IsRegular() {
		return gmailconnector.AttachmentContent{}, fmt.Errorf("path must identify a regular file")
	}
	if info.Size() > remaining {
		return gmailconnector.AttachmentContent{}, fmt.Errorf("attachments exceed the 25 MiB total limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, remaining+1))
	if err != nil {
		return gmailconnector.AttachmentContent{}, err
	}
	if int64(len(data)) > remaining {
		return gmailconnector.AttachmentContent{}, fmt.Errorf("attachments exceed the 25 MiB total limit")
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return gmailconnector.AttachmentContent{
		Attachment: gmailconnector.Attachment{FileName: filepath.Base(path), MIMEType: mimeType, Size: int64(len(data))},
		Data:       data,
	}, nil
}
