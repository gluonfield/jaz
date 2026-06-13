package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wins/jaz/backend/internal/pathsafe"
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
	cwd := optionalCwd(session)
	if cwd == "" {
		return "", "", fmt.Errorf("session has no working directory")
	}
	path, err := filePathFromRequest(raw)
	if err != nil {
		return "", "", err
	}
	roots := sessionFileRoots(session, cwd)
	var last error
	for _, root := range roots {
		abs, err := resolveFileUnderRoot(root, path)
		if err != nil {
			last = err
			continue
		}
		rel, _ := filepath.Rel(cwd, abs)
		if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			rel = ""
		}
		return abs, filepath.ToSlash(rel), nil
	}
	if last != nil {
		return "", "", last
	}
	return "", "", fmt.Errorf("path is outside the session workspace: %s", raw)
}

func sessionFileRoots(session storage.Session, cwd string) []string {
	roots := []string{cwd}
	if session.RuntimeRef != nil {
		if project := strings.TrimSpace(session.RuntimeRef.ProjectPath); project != "" && project != cwd {
			roots = append(roots, project)
		}
	}
	return roots
}

func resolveFileUnderRoot(root, raw string) (string, error) {
	root = filepath.Clean(root)
	abs, err := pathsafe.Resolve(root, raw)
	if err != nil {
		return "", err
	}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realAbs, absErr := filepath.EvalSymlinks(abs)
	if rootErr == nil && absErr == nil && !pathsafe.Within(realRoot, realAbs) {
		return "", fmt.Errorf("path escapes the allowed directory: %s", raw)
	}
	return abs, nil
}

func filePathFromRequest(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "file://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		raw = parsed.Path
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	return raw, nil
}
