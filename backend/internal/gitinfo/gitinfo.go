// Package gitinfo inspects the git state of a working directory — current
// branch, origin remote, forge coordinates — so the UI can offer repo-aware
// actions like opening a pull request.
package gitinfo

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Info is the JSON shape served to the frontend; the zero value means "not a
// git repository".
type Info struct {
	Git           bool   `json:"git"`
	Branch        string `json:"branch,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	RemoteURL     string `json:"remote_url,omitempty"`
	// WebURL is the https page of the origin remote (no trailing slash),
	// present only when the remote parses as host/owner/repo.
	WebURL string `json:"web_url,omitempty"`
	Host   string `json:"host,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
	// HasUpstream reports whether the current branch tracks a remote branch,
	// i.e. it has been pushed somewhere a pull request could start from.
	HasUpstream bool `json:"has_upstream,omitempty"`
	// NoCommits is set when the branch is confirmed to have zero commits on
	// top of the default branch — a PR opened from it would be empty.
	NoCommits bool `json:"no_commits,omitempty"`
	// NeedsPush reports commits the remote doesn't have: an unpushed branch
	// with its own commits, or an upstream the branch is ahead of.
	NeedsPush bool `json:"needs_push,omitempty"`
	// Dirty reports uncommitted changes in the working tree.
	Dirty bool `json:"dirty,omitempty"`
	// IsWorktree marks a linked worktree (not the repository's main checkout);
	// MainBranch is the branch checked out in the main checkout, the handoff
	// destination.
	IsWorktree bool   `json:"is_worktree,omitempty"`
	MainBranch string `json:"main_branch,omitempty"`
	// Behind counts commits on MainBranch the worktree's branch doesn't
	// have yet — what an update would pull in (0 when up to date).
	Behind int `json:"behind,omitempty"`
	// WorktreeMissing reports a managed worktree path that no longer exists.
	WorktreeMissing bool `json:"worktree_missing,omitempty"`
	// WorktreeRestorable reports whether the saved session branch can recreate
	// the missing worktree.
	WorktreeRestorable bool   `json:"worktree_restorable,omitempty"`
	WorktreeBranch     string `json:"worktree_branch,omitempty"`
}

// Inspect collects Info for dir. It never errors: a non-repo (or missing git
// binary) yields the zero Info, and each field degrades independently.
func Inspect(ctx context.Context, dir string) Info {
	if _, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return Info{}
	}
	info := Info{Git: true}
	// symbolic-ref works on an unborn branch (fresh repo, no commits) where
	// rev-parse HEAD does not; it errors when HEAD is detached, leaving "".
	info.Branch, _ = git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if _, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		info.HasUpstream = true
	}
	// Local state first — it must survive the no-remote return below, since
	// commit and handoff don't need a remote.
	if out, err := git(ctx, dir, "status", "--porcelain"); err == nil && out != "" {
		info.Dirty = true
	}
	if _, branch, ok := mainCheckout(ctx, dir); ok {
		info.IsWorktree = true
		info.MainBranch = branch
		if branch != "" {
			if count, err := git(ctx, dir, "rev-list", "--count", "HEAD..refs/heads/"+branch); err == nil {
				info.Behind, _ = strconv.Atoi(count)
			}
		}
	}
	remote, err := git(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		// No origin — fall back to the first configured remote, if any.
		remote = firstRemoteURL(ctx, dir)
	}
	if remote == "" {
		return info
	}
	info.RemoteURL = remote
	info.Host, info.Owner, info.Repo = ParseRemote(remote)
	if info.Host != "" {
		info.WebURL = "https://" + info.Host + "/" + info.Owner + "/" + info.Repo
	}
	info.DefaultBranch = defaultBranch(ctx, dir)
	if info.Branch != "" && info.DefaultBranch != "" && info.Branch != info.DefaultBranch {
		// Prefer the local default branch as base (worktrees always have it);
		// fall back to the remote-tracking ref for clones without one.
		for _, base := range []string{"refs/heads/" + info.DefaultBranch, "refs/remotes/origin/" + info.DefaultBranch} {
			if count, err := git(ctx, dir, "rev-list", "--count", base+"..HEAD"); err == nil {
				info.NoCommits = count == "0"
				break
			}
		}
	}
	if info.Branch != "" {
		if info.HasUpstream {
			if count, err := git(ctx, dir, "rev-list", "--count", "@{upstream}..HEAD"); err == nil && count != "0" {
				info.NeedsPush = true
			}
		} else if !info.NoCommits {
			// Unpushed branch: publishable once it has commits of its own.
			// (NoCommits stays false when the base is unknown — then err on
			// the side of offering the push.)
			info.NeedsPush = true
		}
	}
	return info
}

// mainCheckout resolves the repository's main checkout when dir is a linked
// worktree: the common git dir lives at <main>/.git while the worktree's own
// git dir is elsewhere. ok is false when dir is the main checkout itself.
func mainCheckout(ctx context.Context, dir string) (root, branch string, ok bool) {
	gitDir, err := git(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", "", false
	}
	common, err := git(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", "", false
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	common = filepath.Clean(common)
	// git prints realpaths while dir may arrive through a symlink (macOS
	// /tmp); resolve both sides or a main checkout looks like a worktree.
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	if resolved, err := filepath.EvalSymlinks(gitDir); err == nil {
		gitDir = resolved
	}
	if gitDir == common {
		return "", "", false
	}
	root = filepath.Dir(common)
	branch, _ = git(ctx, root, "symbolic-ref", "--short", "-q", "HEAD")
	return root, branch, true
}

// CommitAll stages everything in dir and commits it with message; a clean
// tree is a no-op rather than an error, so callers can chain it freely.
func CommitAll(ctx context.Context, dir, message string) error {
	out, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	_, err = git(ctx, dir, "commit", "-m", message)
	return err
}

func snapshotAll(ctx context.Context, dir, message string) error {
	out, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if out == "" {
		return nil
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	_, err = gitWithOptions(ctx, dir, []string{
		"-c", "user.email=jaz@local",
		"-c", "user.name=Jaz",
		"-c", "commit.gpgsign=false",
	}, "commit", "--no-verify", "-m", message)
	return err
}

// Handoff applies the worktree's changes — committed and uncommitted,
// relative to where it branched off — onto the repository's main checkout as
// committed history: any dirty work in the worktree is committed on its
// branch first (with message), then that branch is merged into the branch the
// main checkout has checked out — fast-forward when main hasn't moved, a
// merge commit otherwise. A conflicting merge is aborted and reported rather
// than left half-done, so the main checkout never ends up mid-merge. Returns
// the main checkout path.
func MergeIntoMain(ctx context.Context, dir, message string) (string, error) {
	root, branch, ok := mainCheckout(ctx, dir)
	if !ok {
		return "", fmt.Errorf("the session is not running on a worktree")
	}
	if branch == "" {
		return "", fmt.Errorf("the main checkout is on a detached HEAD")
	}
	worktreeBranch, _ := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if worktreeBranch == "" {
		return "", fmt.Errorf("the worktree is on a detached HEAD")
	}
	if err := CommitAll(ctx, dir, message); err != nil {
		return "", err
	}
	if count, err := git(ctx, root, "rev-list", "--count", branch+".."+worktreeBranch); err == nil && count == "0" {
		return "", fmt.Errorf("nothing to merge — %s already has everything from %s", branch, worktreeBranch)
	}
	if _, err := git(ctx, root, "merge", "--no-edit", worktreeBranch); err != nil {
		// A conflicted merge leaves MERGE_HEAD state behind; abort it so the
		// main checkout stays pristine. (Refusals — e.g. local changes would
		// be overwritten — never start a merge; the abort then is a no-op.)
		_, _ = git(ctx, root, "merge", "--abort")
		return "", fmt.Errorf("merging %s into %s: %w", worktreeBranch, branch, err)
	}
	return root, nil
}

func MergeFromMain(ctx context.Context, dir, message string) error {
	_, branch, ok := mainCheckout(ctx, dir)
	if !ok {
		return fmt.Errorf("the session is not running on a worktree")
	}
	if branch == "" {
		return fmt.Errorf("the main checkout is on a detached HEAD")
	}
	worktreeBranch, _ := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if worktreeBranch == "" {
		return fmt.Errorf("the worktree is on a detached HEAD")
	}
	source := "refs/heads/" + branch
	if err := CommitAll(ctx, dir, message); err != nil {
		return err
	}
	if count, err := git(ctx, dir, "rev-list", "--count", worktreeBranch+".."+source); err == nil && count == "0" {
		return fmt.Errorf("nothing to merge — %s already has everything from %s", worktreeBranch, branch)
	}
	if _, err := git(ctx, dir, "merge", "--no-edit", source); err != nil {
		// Abort a conflicted merge so the worktree never sits mid-merge.
		_, _ = git(ctx, dir, "merge", "--abort")
		return fmt.Errorf("merging %s into %s: %w", branch, worktreeBranch, err)
	}
	return nil
}

// Push publishes the current branch to the repository's remote (origin, or
// the first configured remote) with -u, so a pull request can be opened from
// it and later pushes need no flags.
func Push(ctx context.Context, dir string) error {
	remote := "origin"
	if _, err := git(ctx, dir, "remote", "get-url", remote); err != nil {
		names, err := git(ctx, dir, "remote")
		if err != nil || names == "" {
			return fmt.Errorf("no git remote configured")
		}
		name, _, _ := strings.Cut(names, "\n")
		remote = strings.TrimSpace(name)
	}
	_, err := git(ctx, dir, "push", "-u", remote, "HEAD")
	return err
}

func BranchExists(ctx context.Context, dir, branch string) bool {
	if strings.TrimSpace(branch) == "" {
		return false
	}
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func RemoveManagedWorktree(ctx context.Context, dir, branch, message string) error {
	root, _, ok := mainCheckout(ctx, dir)
	if !ok {
		return fmt.Errorf("the session is not running on a worktree")
	}
	current, _ := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if current == "" {
		return fmt.Errorf("the worktree is on a detached HEAD")
	}
	if current != branch {
		return fmt.Errorf("worktree branch %q does not match %q", current, branch)
	}
	if err := snapshotAll(ctx, dir, message); err != nil {
		return err
	}
	if _, err := git(ctx, root, "worktree", "remove", "--force", dir); err != nil {
		return fmt.Errorf("remove worktree: %w", err)
	}
	_, _ = git(ctx, root, "worktree", "prune")
	return nil
}

func RestoreManagedWorktree(ctx context.Context, repo, worktree, branch string) error {
	root, err := git(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("restore worktree requires a git repository at %s: %w", repo, err)
	}
	if !BranchExists(ctx, root, branch) {
		return fmt.Errorf("worktree branch %q was not found", branch)
	}
	if _, err := os.Stat(worktree); err == nil {
		return fmt.Errorf("worktree already exists: %s", worktree)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return err
	}
	_, _ = git(ctx, root, "worktree", "prune")
	if _, err := git(ctx, root, "worktree", "add", worktree, branch); err != nil {
		return fmt.Errorf("restore worktree: %w", err)
	}
	return nil
}

func PruneWorktreeMetadata(ctx context.Context, repo string) error {
	root, err := git(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	_, err = git(ctx, root, "worktree", "prune")
	return err
}

func firstRemoteURL(ctx context.Context, dir string) string {
	names, err := git(ctx, dir, "remote")
	if err != nil || names == "" {
		return ""
	}
	name, _, _ := strings.Cut(names, "\n")
	url, _ := git(ctx, dir, "remote", "get-url", strings.TrimSpace(name))
	return url
}

// defaultBranch resolves the remote's default branch: origin/HEAD when the
// clone recorded it, otherwise whichever of main/master exists on origin.
func defaultBranch(ctx context.Context, dir string) string {
	if ref, err := git(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(ref, "origin/")
	}
	for _, name := range []string{"main", "master"} {
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+name); err == nil {
			return name
		}
	}
	return ""
}

// ParseRemote extracts (host, owner, repo) from a git remote URL in https,
// ssh://, or scp-like (git@host:owner/repo.git) form. Forges with nested
// namespaces keep everything but the last path segment as the owner.
// Unparseable remotes (local paths, bare names) yield empty strings.
func ParseRemote(remote string) (host, owner, repo string) {
	remote = strings.TrimSpace(remote)
	var path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return "", "", ""
		}
		host = u.Hostname()
		path = u.Path
	} else {
		// scp-like: [user@]host:path — a remote without a colon is a local path.
		rest := remote
		if _, after, found := strings.Cut(remote, "@"); found {
			rest = after
		}
		var ok bool
		host, path, ok = strings.Cut(rest, ":")
		if !ok || host == "" || strings.Contains(host, "/") {
			return "", "", ""
		}
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return "", "", ""
	}
	owner = strings.Join(segments[:len(segments)-1], "/")
	repo = segments[len(segments)-1]
	if owner == "" || repo == "" {
		return "", "", ""
	}
	return host, owner, repo
}

// AddWorktree creates a disposable git worktree of the repository containing
// dir at workspace/.worktrees/<slug>, on a fresh "jaz/<slug>" branch. baseRef
// selects the branch point; empty uses dir's current HEAD.
func AddWorktree(ctx context.Context, workspace, dir, slug, baseRef string) (string, string, error) {
	repo, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("worktree requires a git repository at %s: %w", dir, err)
	}
	projectPath := repo
	if root, _, ok := mainCheckout(ctx, dir); ok {
		projectPath = root
	}
	worktree := filepath.Join(workspace, ".worktrees", slug)
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return "", "", err
	}
	args := []string{"worktree", "add", "-b", "jaz/" + slug, worktree}
	if baseRef = strings.TrimSpace(baseRef); baseRef != "" {
		args = append(args, baseRef)
	}
	if _, err := git(ctx, repo, args...); err != nil {
		return "", "", fmt.Errorf("create worktree: %w", err)
	}
	return worktree, projectPath, nil
}

// ListFiles returns the repository's non-ignored files under dir (tracked
// plus untracked), slash-separated and relative to dir — the same view of the
// tree a `.gitignore`-respecting walker would produce. Errors when dir is not
// inside a git working tree (or git is unavailable); callers fall back to a
// plain filesystem walk.
func ListFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := git(ctx, dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	return splitNul(out), nil
}

// splitNul splits NUL-terminated git output (-z flags) into its non-empty
// records.
func splitNul(out string) []string {
	parts := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	fields := parts[:0]
	for _, part := range parts {
		if part != "" {
			fields = append(fields, part)
		}
	}
	return fields
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	return gitWithOptions(ctx, dir, nil, args...)
}

func gitWithOptions(ctx context.Context, dir string, options []string, args ...string) (string, error) {
	gitArgs := append([]string{"-C", dir}, options...)
	gitArgs = append(gitArgs, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Surface git's own message (e.g. "branch already exists") over the
		// bare exit status; probes that expect failure just discard it.
		detail := strings.TrimSpace(out.String())
		if extra := strings.TrimSpace(stderr.String()); extra != "" {
			if detail != "" {
				detail += "\n"
			}
			detail += extra
		}
		if detail != "" {
			return "", fmt.Errorf("git %s: %s", args[0], detail)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
