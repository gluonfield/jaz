package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// FileChange is one changed path in a session's working tree relative to the
// session's diff base (see diffBase).
type FileChange struct {
	Path string `json:"path"`
	// OldPath is the rename source when Status is "renamed".
	OldPath string `json:"old_path,omitempty"`
	Status  string `json:"status"` // added|modified|deleted|renamed|untracked
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	// Binary files carry no line counts.
	Binary bool `json:"binary,omitempty"`
}

// ChangeSummary is the numstat-level view of a session's work: which files
// changed and by how many lines, without any patch text.
type ChangeSummary struct {
	Base         string       `json:"base,omitempty"` // resolved base commit
	Files        []FileChange `json:"files"`
	TotalAdded   int          `json:"total_added"`
	TotalDeleted int          `json:"total_deleted"`
}

// FilePatch is one file's unified diff, served on demand so the UI never
// pulls patch text for files nobody opened.
type FilePatch struct {
	Path      string `json:"path"`
	Patch     string `json:"patch"`
	Binary    bool   `json:"binary,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// FileDiffSpec identifies one changed file the way the summary reported it,
// so the patch matches the row the user opened instead of re-deriving state
// from a tree that may have moved on. The same path can be two rows at once
// — tracked-deleted and recreated untracked — which only the caller can
// distinguish.
type FileDiffSpec struct {
	Path    string
	OldPath string // rename source, diffed together with Path
	// Base pins the diff to the summary's resolved base; empty re-resolves.
	Base string
	// Untracked selects the /dev/null diff — `git diff <base>` can't see
	// untracked files.
	Untracked bool
}

// maxPatchBytes caps a single file's patch; the UI shows a truncation notice
// past this point rather than shipping a megabyte diff nobody scrolls.
const maxPatchBytes = 200 << 10

// maxCountBytes caps how much of an untracked file is read to count its
// lines; beyond it the count is approximate, which the summary can live with.
const maxCountBytes = 1 << 20

// diffBase resolves the commit a session's work is measured against. For a
// worktree that is where its branch forked off the main checkout's branch —
// committed and uncommitted session work both show, and the base stays put as
// main moves on. In a shared checkout only uncommitted work is attributable
// to the session, so HEAD. An unborn HEAD falls back to the empty tree
// (everything shows as added); "" means dir is not a repository.
func diffBase(ctx context.Context, dir string) string {
	if _, branch, ok := mainCheckout(ctx, dir); ok && branch != "" {
		if base, err := git(ctx, dir, "merge-base", "HEAD", branch); err == nil {
			return base
		}
	}
	if head, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		return head
	}
	if empty, err := git(ctx, dir, "hash-object", "-t", "tree", os.DevNull); err == nil {
		return empty
	}
	return ""
}

// DiffSummary lists everything the session changed relative to diffBase:
// tracked changes via numstat, plus untracked files counted in-process —
// `git diff` cannot see those, and `git add -N` would mutate the index.
func DiffSummary(ctx context.Context, dir string) (ChangeSummary, error) {
	base := diffBase(ctx, dir)
	if base == "" {
		return ChangeSummary{}, fmt.Errorf("not a git repository: %s", dir)
	}
	numstat, err := git(ctx, dir, "diff", "--numstat", "-z", "-M", base)
	if err != nil {
		return ChangeSummary{}, err
	}
	nameStatus, err := git(ctx, dir, "diff", "--name-status", "-z", "-M", base)
	if err != nil {
		return ChangeSummary{}, err
	}
	summary := ChangeSummary{Base: base, Files: parseNumstat(numstat, parseNameStatus(nameStatus))}
	untracked, err := git(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return ChangeSummary{}, err
	}
	for _, path := range splitNul(untracked) {
		// The only non-git work in here; honor the caller's deadline like
		// every git call does, or a tree full of large untracked files
		// busts the handler's timeout uncancellably.
		if err := ctx.Err(); err != nil {
			return ChangeSummary{}, err
		}
		lines, binary := countFileLines(filepath.Join(dir, filepath.FromSlash(path)))
		summary.Files = append(summary.Files, FileChange{Path: path, Status: "untracked", Added: lines, Binary: binary})
	}
	sort.Slice(summary.Files, func(i, j int) bool {
		// Status tiebreak: one path can be two rows (deleted + recreated
		// untracked); without it the unstable sort lets them swap between
		// refreshes.
		if summary.Files[i].Path != summary.Files[j].Path {
			return summary.Files[i].Path < summary.Files[j].Path
		}
		return summary.Files[i].Status < summary.Files[j].Status
	})
	for _, file := range summary.Files {
		summary.TotalAdded += file.Added
		summary.TotalDeleted += file.Deleted
	}
	return summary, nil
}

// FileDiff returns one file's unified diff as identified by spec.
func FileDiff(ctx context.Context, dir string, spec FileDiffSpec) (FilePatch, error) {
	var out string
	var err error
	if spec.Untracked {
		out, err = gitDiff(ctx, dir, "diff", "--no-index", "--", os.DevNull, spec.Path)
	} else {
		base := spec.Base
		if base == "" {
			base = diffBase(ctx, dir)
		}
		if base == "" {
			return FilePatch{}, fmt.Errorf("not a git repository: %s", dir)
		}
		args := []string{"diff", "-M", base, "--", spec.Path}
		if spec.OldPath != "" {
			args = append(args, spec.OldPath)
		}
		out, err = gitDiff(ctx, dir, args...)
	}
	if err != nil {
		return FilePatch{}, err
	}
	patch := FilePatch{Path: spec.Path, Patch: out}
	// Backstop for binaries the summary didn't flag: the change emits
	// "Binary files ... differ" and no hunks.
	if !strings.Contains(out, "\n@@") && strings.Contains(out, "Binary files ") {
		patch.Binary = true
		patch.Patch = ""
	}
	if len(patch.Patch) > maxPatchBytes {
		cut := strings.LastIndexByte(patch.Patch[:maxPatchBytes], '\n')
		if cut < 0 {
			cut = maxPatchBytes
		}
		patch.Patch = patch.Patch[:cut]
		patch.Truncated = true
	}
	return patch, nil
}

// gitDiff runs a git diff command and returns the patch bytes intact —
// git()'s TrimSpace would eat trailing blank context lines off the last
// hunk. Exit status 1 is success here: the diff family uses it for "files
// differ" (--no-index always does).
func gitDiff(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("git diff: %s", detail)
		}
		return "", err
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

// parseNumstat decodes `git diff --numstat -z -M` records — regular entries
// as one "added\tdeleted\tpath" token, renames as an "added\tdeleted\t"
// token followed by the old and new paths as two more tokens. status maps
// each (new) path to its name-status letter.
func parseNumstat(out string, status map[string]string) []FileChange {
	files := []FileChange{}
	tokens := splitNul(out)
	for len(tokens) > 0 {
		added, rest, ok := strings.Cut(tokens[0], "\t")
		if !ok {
			break
		}
		deleted, path, _ := strings.Cut(rest, "\t")
		change := FileChange{Path: path}
		if path != "" {
			tokens = tokens[1:]
		} else {
			if len(tokens) < 3 {
				break
			}
			change.OldPath, change.Path = tokens[1], tokens[2]
			tokens = tokens[3:]
		}
		// Binary files report "-\t-".
		if added == "-" {
			change.Binary = true
		} else {
			change.Added, _ = strconv.Atoi(added)
			change.Deleted, _ = strconv.Atoi(deleted)
		}
		change.Status = changeStatus(status[change.Path], change.OldPath != "")
		files = append(files, change)
	}
	return files
}

// parseNameStatus decodes `git diff --name-status -z -M` into path → status
// letter. Rename and copy records carry two path tokens; the new path keys.
func parseNameStatus(out string) map[string]string {
	tokens := splitNul(out)
	status := make(map[string]string, len(tokens)/2)
	for len(tokens) >= 2 {
		letter, path := tokens[0], tokens[1]
		if strings.HasPrefix(letter, "R") || strings.HasPrefix(letter, "C") {
			if len(tokens) < 3 {
				break
			}
			path = tokens[2]
			tokens = tokens[3:]
		} else {
			tokens = tokens[2:]
		}
		status[path] = letter
	}
	return status
}

func changeStatus(letter string, renamed bool) string {
	switch {
	case strings.HasPrefix(letter, "R") || renamed:
		return "renamed"
	case letter == "A" || strings.HasPrefix(letter, "C"):
		return "added"
	case letter == "D":
		return "deleted"
	default:
		return "modified"
	}
}

// countFileLines counts an untracked file's lines (capped — see
// maxCountBytes) and sniffs binaries the way git does: a NUL byte in the
// first 8000 bytes.
func countFileLines(path string) (lines int, binary bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCountBytes))
	if err != nil || len(data) == 0 {
		return 0, false
	}
	sniff := data
	if len(sniff) > 8000 {
		sniff = sniff[:8000]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return 0, true
	}
	lines = bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines, false
}
