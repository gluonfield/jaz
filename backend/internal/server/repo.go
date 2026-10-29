package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/worktrees"
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
	if info, ok := s.missingWorktreeInfo(ctx, session); ok {
		writeJSON(w, http.StatusOK, info)
		return
	}
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
		writeError(w, http.StatusBadGateway, err)
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
	// Pushes go over the network; give them room without hanging forever.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	info, err := s.pushSessionRepo(ctx, session)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type repoCommitRequest struct {
	Message string `json:"message,omitempty"`
}

// handleSessionRepoCommit stages and commits everything in the session's
// working directory. The message defaults to the session title — in a
// worktree the only changes are this session's work, so the title names it.
func (s *Server) handleSessionRepoCommit(w http.ResponseWriter, r *http.Request, session storage.Session) {
	var req repoCommitRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := s.commitSessionRepo(ctx, session, req.Message)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleSessionRepoMerge commits the worktree's outstanding work on its
// branch and merges that branch into the repo's main checkout.
func (s *Server) handleSessionRepoMerge(w http.ResponseWriter, r *http.Request, session storage.Session) {
	var req repoCommitRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	root, info, err := s.mergeSessionRepo(ctx, session, req.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cwd":  root,
		"info": info,
	})
}

func (s *Server) handleSessionRepoMergeFromMain(w http.ResponseWriter, r *http.Request, session storage.Session) {
	var req repoCommitRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := s.mergeFromMainSessionRepo(ctx, session, req.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionRepoRestoreWorktree(w http.ResponseWriter, r *http.Request, session storage.Session) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	info, err := s.restoreSessionWorktree(ctx, session)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) pushSessionRepo(ctx context.Context, session storage.Session) (gitinfo.Info, error) {
	cwd, err := sessionCwdValue(session)
	if err != nil {
		return gitinfo.Info{}, err
	}
	if err := gitinfo.Push(ctx, cwd); err != nil {
		return gitinfo.Info{}, err
	}
	return gitinfo.Inspect(ctx, cwd), nil
}

func (s *Server) commitSessionRepo(ctx context.Context, session storage.Session, rawMessage string) (gitinfo.Info, error) {
	cwd, err := sessionCwdValue(session)
	if err != nil {
		return gitinfo.Info{}, err
	}
	message := repoCommitMessage(session, rawMessage)
	if err := gitinfo.CommitAll(ctx, cwd, message); err != nil {
		return gitinfo.Info{}, err
	}
	return gitinfo.Inspect(ctx, cwd), nil
}

func (s *Server) mergeSessionRepo(ctx context.Context, session storage.Session, rawMessage string) (string, gitinfo.Info, error) {
	cwd, err := sessionCwdValue(session)
	if err != nil {
		return "", gitinfo.Info{}, err
	}
	root, err := gitinfo.MergeIntoMain(ctx, cwd, repoCommitMessage(session, rawMessage))
	if err != nil {
		return "", gitinfo.Info{}, err
	}
	return root, gitinfo.Inspect(ctx, cwd), nil
}

func (s *Server) mergeFromMainSessionRepo(ctx context.Context, session storage.Session, rawMessage string) (gitinfo.Info, error) {
	cwd, err := sessionCwdValue(session)
	if err != nil {
		return gitinfo.Info{}, err
	}
	if err := gitinfo.MergeFromMain(ctx, cwd, repoCommitMessage(session, rawMessage)); err != nil {
		return gitinfo.Info{}, err
	}
	return gitinfo.Inspect(ctx, cwd), nil
}

func (s *Server) restoreSessionWorktree(ctx context.Context, session storage.Session) (gitinfo.Info, error) {
	worktree, ok := s.managedWorktree(session)
	if !ok {
		return gitinfo.Info{}, fmt.Errorf("session is not on a managed worktree")
	}
	if _, err := os.Stat(worktree.Cwd); err == nil {
		return gitinfo.Inspect(ctx, worktree.Cwd), nil
	} else if !os.IsNotExist(err) {
		return gitinfo.Info{}, err
	}
	if worktree.ProjectPath == "" {
		return gitinfo.Info{}, fmt.Errorf("session has no project path to restore from")
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return gitinfo.Info{}, err
	}
	return gitinfo.Inspect(ctx, worktree.Cwd), nil
}

func repoCommitMessage(session storage.Session, raw string) string {
	return firstNonEmpty(strings.TrimSpace(raw), strings.TrimSpace(session.Title), "jaz session changes")
}

func (s *Server) PruneManagedWorktrees(ctx context.Context) {
	s.worktreePruneMu.Lock()
	defer s.worktreePruneMu.Unlock()
	pruner := worktrees.Pruner{
		Store:     s.Store,
		Workspace: s.Workspace,
		Runtime:   s.ACP,
		Lock:      s.lockSession,
		Log:       s.logger().WithPrefix("worktrees"),
	}
	if err := pruner.Prune(ctx); err != nil {
		s.logger().WithPrefix("worktrees").Warn("prune failed", "error", err)
	}
}

func (s *Server) managedWorktree(session storage.Session) (worktrees.Worktree, bool) {
	return worktrees.Managed(s.Workspace, session)
}

func (s *Server) missingWorktreeInfo(ctx context.Context, session storage.Session) (gitinfo.Info, bool) {
	worktree, ok := s.managedWorktree(session)
	if !ok {
		return gitinfo.Info{}, false
	}
	if _, err := os.Stat(worktree.Cwd); err == nil || !os.IsNotExist(err) {
		return gitinfo.Info{}, false
	}
	if owner, err := worktrees.Owner(s.Store, s.Workspace, worktree); err == nil {
		worktree = owner
	}
	info := gitinfo.Info{
		Revision:        gitinfo.Revision{Branch: worktree.Branch},
		WorktreeMissing: true,
		WorktreeBranch:  worktree.Branch,
	}
	if worktree.ProjectPath != "" && gitinfo.BranchExists(ctx, worktree.ProjectPath, worktree.Branch) {
		info.WorktreeRestorable = true
	}
	return info, true
}

func (s *Server) ensureManagedWorktree(ctx context.Context, session storage.Session) error {
	return worktrees.Ensure(ctx, s.Store, s.Workspace, session)
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
	cwd, err := sessionCwdValue(session)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return "", false
	}
	return cwd, true
}

func sessionCwdValue(session storage.Session) (string, error) {
	cwd := optionalCwd(session)
	if cwd == "" {
		return "", fmt.Errorf("session has no working directory")
	}
	return cwd, nil
}
