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
		ServerPath: abs,
	}
	if err := writeAttachmentMetadata(dir, attachment); err != nil {
		_ = os.Remove(path)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, attachmentResponseFromStorage(attachment))
}

func (s *Server) handleAttachmentContent(w http.ResponseWriter, r *http.Request, session storage.Session, id string) {
	id = strings.TrimSpace(id)
	if !validAttachmentID(id) {
		writeError(w, http.StatusNotFound, fmt.Errorf("attachment not found"))
		return
	}
	attachments, err := s.resolveAttachments(session.ID, []string{id})
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
