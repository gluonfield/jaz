package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
