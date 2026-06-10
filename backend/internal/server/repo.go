package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
)

// handleSessionRepo reports the git/forge state of the session's working
// directory so the titlebar can offer repo actions (create PR, open repo).
// Sessions without a cwd report the zero Info rather than an error.
func (s *Server) handleSessionRepo(w http.ResponseWriter, r *http.Request, session storage.Session) {
	cwd := ""
	if session.RuntimeRef != nil {
		cwd = strings.TrimSpace(session.RuntimeRef.Cwd)
	}
	if cwd == "" {
		writeJSON(w, http.StatusOK, gitinfo.Info{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, gitinfo.Inspect(ctx, cwd))
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

func sessionCwd(w http.ResponseWriter, session storage.Session) (string, bool) {
	cwd := ""
	if session.RuntimeRef != nil {
		cwd = strings.TrimSpace(session.RuntimeRef.Cwd)
	}
	if cwd == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("session has no working directory"))
		return "", false
	}
	return cwd, true
}
