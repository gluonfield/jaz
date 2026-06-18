package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
)

const sessionFileReadLimit = 1024 * 1024

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
