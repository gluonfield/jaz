package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	projectSettingNamespace = "projects"
	projectSettingKey       = "list"
)

type project struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Git  bool   `json:"git"`
}

type filesystemDir struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Git  bool   `json:"git"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	paths, err := s.loadProjectPaths()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	projects := make([]project, 0, len(paths))
	for _, path := range paths {
		p, err := projectFromPath(path)
		if err == nil {
			projects = append(projects, p)
		}
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name != projects[j].Name {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].Path < projects[j].Path
	})
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	p, err := projectFromPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	paths, err := s.loadProjectPaths()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	found := false
	for _, path := range paths {
		if path == p.Path {
			found = true
			break
		}
	}
	if !found {
		paths = append(paths, p.Path)
		if err := s.saveProjectPaths(paths); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleListFilesystemDirs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		path = firstNonEmpty(serverHomeDir(), s.Workspace, ".")
	}
	abs, err := cleanExistingDir(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dirs, err := pathsafe.Subdirs(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out := make([]filesystemDir, 0, len(dirs))
	for _, dir := range dirs {
		path := filepath.Join(abs, dir.Name)
		out = append(out, filesystemDir{Name: dir.Name, Path: path, Git: dir.Git})
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":   abs,
		"parent": parent,
		"git":    pathsafe.IsGitRepo(abs),
		"dirs":   out,
	})
}

func (s *Server) resolveWorkingDir(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if filepath.IsAbs(directory) {
		return cleanExistingDir(directory)
	}
	return s.resolveWorkspaceDir(directory)
}

func (s *Server) resolveWorkspaceFileRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return cleanExistingDir(path)
	}
	if strings.TrimSpace(s.Workspace) == "" {
		return "", fmt.Errorf("workspace is not configured")
	}
	abs, err := pathsafe.Resolve(s.Workspace, path)
	if err != nil {
		return "", err
	}
	return cleanExistingDir(abs)
}

func (s *Server) loadProjectPaths() ([]string, error) {
	setting, err := s.Store.LoadSetting(projectSettingNamespace, projectSettingKey)
	if err != nil {
		if errors.Is(err, storage.ErrSettingNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(setting.Value, &paths); err != nil {
		return nil, err
	}
	return dedupeStrings(paths), nil
}

func (s *Server) saveProjectPaths(paths []string) error {
	data, err := json.Marshal(dedupeStrings(paths))
	if err != nil {
		return err
	}
	_, err = s.Store.SaveSetting(projectSettingNamespace, projectSettingKey, data)
	return err
}

func projectFromPath(path string) (project, error) {
	abs, err := cleanExistingDir(path)
	if err != nil {
		return project{}, err
	}
	return project{Name: projectName(abs), Path: abs, Git: pathsafe.IsGitRepo(abs)}, nil
}

func projectPathForRequest(directory, cwd string) string {
	if strings.TrimSpace(directory) == "." {
		return ""
	}
	return cwd
}

func projectName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}

func cleanExistingDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", abs)
	}
	return abs, nil
}

func serverHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
