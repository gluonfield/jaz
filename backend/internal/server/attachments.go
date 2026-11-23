package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

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

	attachment, status, err := s.storeAttachment(session.ID, safeAttachmentName(header.Filename), header.Header.Get("Content-Type"), file, maxBytes)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, attachmentResponseFromStorage(attachment))
}

func (s *Server) SaveAttachment(sessionID, name string, source io.Reader) (storage.Attachment, error) {
	attachment, _, err := s.storeAttachment(sessionID, name, "", source, attachmentMaxBytes())
	return attachment, err
}

func (s *Server) storeAttachment(sessionID, name, contentType string, source io.Reader, maxBytes int64) (storage.Attachment, int, error) {
	root, err := s.attachmentRoot()
	if err != nil {
		return storage.Attachment{}, http.StatusInternalServerError, err
	}
	id, err := newAttachmentID()
	if err != nil {
		return storage.Attachment{}, http.StatusInternalServerError, err
	}
	dir := filepath.Join(root, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return storage.Attachment{}, http.StatusInternalServerError, err
	}
	path := filepath.Join(dir, id+"-"+safeAttachmentName(name))
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return storage.Attachment{}, http.StatusInternalServerError, err
	}
	n, copyErr := io.Copy(out, io.LimitReader(source, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return storage.Attachment{}, http.StatusBadRequest, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return storage.Attachment{}, http.StatusInternalServerError, closeErr
	}
	if n > maxBytes {
		_ = os.Remove(path)
		return storage.Attachment{}, http.StatusRequestEntityTooLarge, fmt.Errorf("attachment exceeds %d bytes", maxBytes)
	}

	attachment := storage.Attachment{
		ID:         id,
		Name:       name,
		MimeType:   attachmentMimeType(contentType, name),
		Size:       n,
		ServerPath: path,
	}
	if err := writeAttachmentMetadata(dir, attachment); err != nil {
		_ = os.Remove(path)
		_ = os.Remove(filepath.Join(dir, id+".json"))
		return storage.Attachment{}, http.StatusInternalServerError, err
	}
	return attachment, http.StatusOK, nil
}

func (s *Server) handleAttachmentContent(w http.ResponseWriter, r *http.Request, session storage.Session, id string) {
	id = strings.TrimSpace(id)
	if !validAttachmentID(id) {
		writeError(w, http.StatusNotFound, fmt.Errorf("attachment not found"))
		return
	}
	attachments, err := s.ResolveAttachments(session.ID, []string{id})
	if err != nil || len(attachments) != 1 {
		writeError(w, http.StatusNotFound, fmt.Errorf("attachment not found"))
		return
	}
	attachment := attachments[0]
	info, err := os.Stat(attachment.ServerPath)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, fmt.Errorf("attachment not found"))
		return
	}
	serveAttachmentContent(w, r, attachment, info)
}

func serveAttachmentContent(w http.ResponseWriter, r *http.Request, attachment storage.Attachment, info os.FileInfo) {
	file, err := os.Open(attachment.ServerPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	ctype := attachmentContentType(attachment)
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	dispositionType := "attachment"
	if inlineAttachmentContentType(ctype) {
		dispositionType = "inline"
	}
	if disposition := mime.FormatMediaType(dispositionType, map[string]string{"filename": attachment.Name}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, attachment.Name, info.ModTime(), file)
}

func attachmentContentType(attachment storage.Attachment) string {
	if ctype := strings.TrimSpace(attachment.MimeType); ctype != "" && !strings.EqualFold(ctype, "application/octet-stream") {
		return ctype
	}
	if byExt := mime.TypeByExtension(filepath.Ext(attachment.Name)); byExt != "" {
		return byExt
	}
	return strings.TrimSpace(attachment.MimeType)
}

func inlineAttachmentContentType(ctype string) bool {
	mediaType, _, err := mime.ParseMediaType(ctype)
	if err != nil {
		mediaType = ctype
	}
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/avif", "image/bmp", "image/gif", "image/heic", "image/heif", "image/jpeg", "image/png", "image/tiff", "image/webp":
		return true
	default:
		return false
	}
}
