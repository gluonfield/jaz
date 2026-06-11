package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

// handleSessionRepo reports the git/forge state of the session's working
// directory so the titlebar can offer repo actions (create PR, open repo).
// Sessions without a cwd report the zero Info rather than an error.
func (s *Server) handleSessionRepo(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd := optionalCwd(session)
	if cwd == "" {
		writeJSON(w, http.StatusOK, gitinfo.Info{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, gitinfo.Inspect(ctx, cwd))
}

// handleSessionRepoChanges reports which files the session changed and by how
// many lines — numstat only, no patch text. The frontend fetches this at turn
// boundaries rather than on an interval, and pulls individual patches through
// handleSessionRepoDiff only when the user opens a file. No cwd or a non-repo
// degrades to an empty summary, mirroring handleSessionRepo.
func (s *Server) handleSessionRepoChanges(w http.ResponseWriter, r *http.Request, session storage.Session) {
	empty := gitinfo.ChangeSummary{Files: []gitinfo.FileChange{}}
	cwd := optionalCwd(session)
	if cwd == "" {
		writeJSON(w, http.StatusOK, empty)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	summary, err := gitinfo.DiffSummary(ctx, cwd)
	if err != nil {
		writeJSON(w, http.StatusOK, empty)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleSessionRepoDiff serves one file's unified diff on demand. The query
// params carry the summary row's identity (status, rename source, resolved
// base) so the patch matches what the row reported — see gitinfo.FileDiffSpec.
// path and old_path end up in `git diff --`, so they must stay relative and
// inside the working directory; base reaches argv, so it must be a hash.
func (s *Server) handleSessionRepoDiff(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd, ok := sessionCwd(w, session)
	if !ok {
		return
	}
	query := r.URL.Query()
	spec := gitinfo.FileDiffSpec{
		Path:      strings.TrimSpace(query.Get("path")),
		OldPath:   strings.TrimSpace(query.Get("old_path")),
		Base:      strings.TrimSpace(query.Get("base")),
		Untracked: query.Get("untracked") == "1",
	}
	if spec.Path == "" || !filepath.IsLocal(spec.Path) || (spec.OldPath != "" && !filepath.IsLocal(spec.OldPath)) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("path must be relative to the session's working directory"))
		return
	}
	if spec.Base != "" && !isHexRev(spec.Base) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("base must be a commit hash"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	patch, err := gitinfo.FileDiff(ctx, cwd, spec)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, patch)
}

// isHexRev accepts only abbreviated-to-full commit hashes, so nothing flag-
// or ref-shaped reaches git argv.
func isHexRev(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// handleSessionRepoPush publishes the session's current branch to its remote
// (git push -u) and returns the refreshed repo state. "Create pull request"
// calls this first when the branch has no upstream yet.
func (s *Server) handleSessionRepoPush(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd, ok := sessionCwd(w, session)
	if !ok {
		return
	}
	// Pushes go over the network; give them room without hanging forever.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := gitinfo.Push(ctx, cwd); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, gitinfo.Inspect(ctx, cwd))
}

type repoCommitRequest struct {
	Message string `json:"message,omitempty"`
}

// handleSessionRepoCommit stages and commits everything in the session's
// working directory. The message defaults to the session title — in a
// worktree the only changes are this session's work, so the title names it.
func (s *Server) handleSessionRepoCommit(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd, ok := sessionCwd(w, session)
	if !ok {
		return
	}
	var req repoCommitRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	message := firstNonEmpty(strings.TrimSpace(req.Message), strings.TrimSpace(session.Title), "jaz session changes")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := gitinfo.CommitAll(ctx, cwd, message); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, gitinfo.Inspect(ctx, cwd))
}

// handleSessionRepoMerge commits the worktree's outstanding work on its
// branch and merges that branch into the repo's main checkout. Native
// sessions follow the work back — their cwd is read fresh each turn — while
// ACP agents stay in the worktree, since their spawned process is bound to it.
func (s *Server) handleSessionRepoMerge(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd, ok := sessionCwd(w, session)
	if !ok {
		return
	}
	var req repoCommitRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	message := firstNonEmpty(strings.TrimSpace(req.Message), strings.TrimSpace(session.Title), "jaz session changes")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	root, err := gitinfo.MergeIntoMain(ctx, cwd, message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	moved := false
	if session.Runtime == "" || session.Runtime == storage.RuntimeNative {
		unlock := s.lockSession(session.ID)
		if current, err := s.Store.LoadSession(session.ID); err == nil {
			session = current
		}
		if session.RuntimeRef != nil {
			ref := *session.RuntimeRef
			ref.Cwd = root
			session.RuntimeRef = &ref
			if err := s.Store.SaveSession(session); err == nil {
				moved = true
			}
		}
		unlock()
	}
	inspectDir := cwd
	if moved {
		inspectDir = root
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":   root,
		"moved": moved,
		"info":  gitinfo.Inspect(ctx, inspectDir),
	})
}

// optionalCwd is for read-only handlers that degrade gracefully without a
// working directory; handlers that require one go through sessionCwd.
func optionalCwd(session storage.Session) string {
	if session.RuntimeRef == nil {
		return ""
	}
	return strings.TrimSpace(session.RuntimeRef.Cwd)
}

func sessionCwd(w http.ResponseWriter, session storage.Session) (string, bool) {
	cwd := optionalCwd(session)
	if cwd == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("session has no working directory"))
		return "", false
	}
	return cwd, true
}
