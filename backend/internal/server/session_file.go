package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
)

const sessionFileReadLimit = 1024 * 1024

// sessionFileWriteLimit caps a save from a file viewer app.
const sessionFileWriteLimit = 10 * 1024 * 1024

type sessionFileResponse struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path,omitempty"`
	Content      string `json:"content,omitempty"`
	Size         int64  `json:"size"`
	Binary       bool   `json:"binary,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
}

func (s *Server) handleSessionFile(w http.ResponseWriter, r *http.Request, session storage.Session) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("path is required"))
		return
	}
	abs, rel, err := resolveSessionFile(session, path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, fmt.Errorf("path is a directory: %s", abs))
		return
	}
	if rawSessionFileRequested(r) {
		serveSessionFileContent(w, r, abs, info)
		return
	}
	file, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, sessionFileReadLimit+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	truncated := len(data) > sessionFileReadLimit
	if truncated {
		data = data[:sessionFileReadLimit]
	}
	binary := bytes.Contains(data, []byte{0}) || !utf8.Valid(data)
	resp := sessionFileResponse{
		Path:         abs,
		RelativePath: rel,
		Size:         info.Size(),
		Binary:       binary,
		Truncated:    truncated,
	}
	if !binary {
		resp.Content = string(data)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleWriteSessionFile replaces an existing session file with the request
// body. With If-Match, it saves only while the file is still that version, so
// an edit made elsewhere in the meantime is refused instead of overwritten.
func (s *Server) handleWriteSessionFile(w http.ResponseWriter, r *http.Request) {
	session, err := s.Store.LoadSession(r.PathValue("session"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	abs, _, err := resolveSessionFile(session, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sessionFileWriteLimit))
	if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]int64{"max_bytes": tooLarge.Limit})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if match := r.Header.Get("If-Match"); match != "" && match != fileETag(info) {
		w.Header().Set("ETag", fileETag(info))
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if err := os.WriteFile(abs, data, info.Mode().Perm()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if info, err = os.Stat(abs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("ETag", fileETag(info))
	w.WriteHeader(http.StatusNoContent)
}

// fileETag versions a file by modification time and size, as HTTP file
// servers do, so knowing a version costs a stat rather than a read.
func fileETag(info os.FileInfo) string {
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size())
}

func rawSessionFileRequested(r *http.Request) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("raw"))
	return raw == "1" || strings.EqualFold(raw, "true")
}

func rawSessionFileContentRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/v1/sessions/") && strings.HasSuffix(r.URL.Path, "/file") && rawSessionFileRequested(r)
}

func serveSessionFileContent(w http.ResponseWriter, r *http.Request, abs string, info os.FileInfo) {
	file, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()
	if ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(abs))); ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(abs)}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("ETag", fileETag(info))
	http.ServeContent(w, r, filepath.Base(abs), info.ModTime(), file)
}

func resolveSessionFile(session storage.Session, raw string) (string, string, error) {
	path, err := filePathFromRequest(raw)
	if err != nil {
		return "", "", err
	}
	cwd := optionalCwd(session)
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		if cwd == "" {
			return "", "", fmt.Errorf("session has no working directory")
		}
		abs = filepath.Join(cwd, path)
	}
	return abs, relativeToCwd(cwd, abs), nil
}

func relativeToCwd(cwd, abs string) string {
	if cwd == "" {
		return ""
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func filePathFromRequest(raw string) (string, error) {
	return filepathx.FromUserInput(raw)
}
