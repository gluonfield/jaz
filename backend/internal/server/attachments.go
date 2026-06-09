package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
)

const defaultAttachmentMaxBytes int64 = 32 << 20

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request, session storage.Session) {
	maxBytes := attachmentMaxBytes()
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid attachment upload: %w", err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("file is required"))
		return
	}
	defer file.Close()

	root, err := s.attachmentRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	id, err := newAttachmentID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	name := safeAttachmentName(header.Filename)
	dir := filepath.Join(root, session.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path := filepath.Join(dir, id+"-"+name)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	n, copyErr := io.Copy(out, io.LimitReader(file, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusBadRequest, copyErr)
		return
	}
	if closeErr != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, closeErr)
		return
	}
	if n > maxBytes {
		_ = os.Remove(path)
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("attachment exceeds %d bytes", maxBytes))
		return
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	attachment := storage.Attachment{
		ID:         id,
		Name:       name,
		MimeType:   attachmentMimeType(header.Header.Get("Content-Type"), name),
		Size:       n,
		URI:        fileURI(abs),
		ServerPath: abs,
	}
	if err := writeAttachmentMetadata(dir, attachment); err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, attachment)
}

func (s *Server) resolveAttachments(sessionID string, ids []string) ([]storage.Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	root, err := s.attachmentRoot()
	if err != nil {
		return nil, err
	}
	out := make([]storage.Attachment, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !validAttachmentID(id) {
			return nil, fmt.Errorf("attachment %s not found for this session", id)
		}
		attachment, err := readAttachmentMetadata(filepath.Join(root, sessionID), id)
		if err != nil {
			return nil, fmt.Errorf("attachment %s not found for this session", id)
		}
		info, err := os.Stat(attachment.ServerPath)
		if err != nil {
			return nil, fmt.Errorf("attachment %s is not readable", id)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("attachment %s is a directory", id)
		}
		attachment.Size = info.Size()
		attachment.URI = fileURI(attachment.ServerPath)
		out = append(out, attachment)
	}
	return out, nil
}

func (s *Server) attachmentRoot() (string, error) {
	base := strings.TrimSpace(s.Workspace)
	if base == "" {
		base = strings.TrimSpace(s.Root)
	}
	if base == "" {
		return "", fmt.Errorf("workspace is not configured")
	}
	root, err := filepath.Abs(filepath.Join(base, ".jaz-attachments"))
	if err != nil {
		return "", err
	}
	return root, nil
}

func attachmentMaxBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("JAZ_ATTACHMENT_MAX_BYTES"))
	if raw == "" {
		return defaultAttachmentMaxBytes
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultAttachmentMaxBytes
	}
	return value
}

func newAttachmentID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func validAttachmentID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func safeAttachmentName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name = strings.Trim(b.String(), ". ")
	if name == "" {
		name = "attachment"
	}
	if len(name) <= 120 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if len(ext) > 24 {
		ext = ""
	}
	limit := 120 - len(ext)
	if limit < 1 {
		limit = 120
	}
	if len(base) > limit {
		base = base[:limit]
	}
	return strings.Trim(base, ". ") + ext
}

func attachmentMimeType(uploaded, name string) string {
	if uploaded = strings.TrimSpace(uploaded); uploaded != "" {
		return uploaded
	}
	if byExt := mime.TypeByExtension(filepath.Ext(name)); byExt != "" {
		return byExt
	}
	return "application/octet-stream"
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func writeAttachmentMetadata(dir string, attachment storage.Attachment) error {
	data, err := json.MarshalIndent(attachment, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, attachment.ID+".json"), data, 0o600)
}

func readAttachmentMetadata(dir, id string) (storage.Attachment, error) {
	if !validAttachmentID(id) {
		return storage.Attachment{}, fmt.Errorf("invalid attachment id")
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return storage.Attachment{}, err
	}
	var attachment storage.Attachment
	if err := json.Unmarshal(data, &attachment); err != nil {
		return storage.Attachment{}, err
	}
	if attachment.ID != id || attachment.ServerPath == "" {
		return storage.Attachment{}, fmt.Errorf("invalid metadata")
	}
	return attachment, nil
}

func appendUserMessage(store storage.MessageAppender, sessionID, message string, attachments []storage.Attachment) error {
	if len(attachments) > 0 {
		if appender, ok := store.(storage.MessageRecordAppender); ok {
			return appender.AppendMessageRecords(sessionID, userMessageRecord(message, attachments))
		}
	}
	return store.AppendMessages(sessionID, provider.UserMessage(message))
}

func userMessageRecord(message string, attachments []storage.Attachment) storage.Message {
	blocks := []storage.Block{{Type: "text", Text: message}}
	for _, attachment := range attachments {
		blocks = append(blocks, attachmentBlock(attachment))
	}
	return storage.Message{
		Role:      "user",
		Content:   message,
		Blocks:    blocks,
		CreatedAt: time.Now().UTC(),
	}
}

func attachmentBlock(attachment storage.Attachment) storage.Block {
	return storage.Block{
		Type:       "attachment",
		ID:         attachment.ID,
		Name:       attachment.Name,
		URI:        attachment.URI,
		MimeType:   attachment.MimeType,
		Size:       attachment.Size,
		ServerPath: attachment.ServerPath,
	}
}

func messageWithAttachmentLinks(message string, attachments []storage.Attachment) string {
	if len(attachments) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n\nAttachments:\n")
	for _, attachment := range attachments {
		fmt.Fprintf(&b, "- %s: %s\n", attachment.Name, attachment.URI)
	}
	return strings.TrimRight(b.String(), "\n")
}
