package applypatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
)

type Tool struct {
	Workspace string
	// ExtraRoots are additional writable directories outside the workspace —
	// the loop automations dir (memory.md, widget/) lives there. The shell can
	// already write anywhere, so this widens convenience, not capability.
	ExtraRoots []string
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		"apply_patch",
		"Use the `apply_patch` tool to edit files. The patch must use the Codex apply_patch format.",
		false,
		tools.ObjectSchema(map[string]any{
			"patch": tools.StringSchema("Patch text starting with *** Begin Patch and ending with *** End Patch."),
		}, []string{"patch"}),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	select {
	case <-ctx.Done():
		return tools.Result{}, ctx.Err()
	default:
	}

	patch := tools.StringInput(inputs, "patch")
	if patch == "" {
		patch = tools.StringInput(inputs, "__freeform")
	}
	if patch == "" {
		return tools.Result{}, errors.New("patch is required")
	}
	hunks, err := parsePatch(patch)
	if err != nil {
		return tools.Result{}, err
	}
	changed := make([]string, 0, len(hunks))
	base, err := sessioncontext.WorkspaceBase(ctx, t.Workspace)
	if err != nil {
		return tools.Result{}, err
	}
	roots := append([]string{base}, t.ExtraRoots...)
	for _, hunk := range hunks {
		if err := applyHunk(roots, hunk); err != nil {
			return tools.Result{}, err
		}
		changed = append(changed, hunk.pathForResult())
	}
	return tools.JSONResult(map[string]any{
		"status":  "completed",
		"changed": changed,
	})
}

type patchKind int

const (
	patchAdd patchKind = iota
	patchDelete
	patchUpdate
)

type patchHunk struct {
	kind   patchKind
	path   string
	moveTo string
	lines  []string
}

func (h patchHunk) pathForResult() string {
	if h.moveTo != "" {
		return h.moveTo
	}
	return h.path
}

func parsePatch(patch string) ([]patchHunk, error) {
	lines := strings.Split(strings.TrimSpace(patch), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" || strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return nil, errors.New("invalid patch boundaries")
	}

	var hunks []patchHunk
	for i := 1; i < len(lines)-1; {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			i++
			body := make([]string, 0)
			for i < len(lines)-1 && !isHunkMarker(lines[i]) {
				if !strings.HasPrefix(lines[i], "+") {
					return nil, fmt.Errorf("add file %s contains non-add line %q", path, lines[i])
				}
				body = append(body, strings.TrimPrefix(lines[i], "+"))
				i++
			}
			hunks = append(hunks, patchHunk{kind: patchAdd, path: path, lines: body})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			hunks = append(hunks, patchHunk{kind: patchDelete, path: path})
			i++
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			i++
			moveTo := ""
			if i < len(lines)-1 && strings.HasPrefix(strings.TrimSpace(lines[i]), "*** Move to: ") {
				moveTo = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "*** Move to: "))
				i++
			}
			body := make([]string, 0)
			for i < len(lines)-1 && !isHunkMarker(lines[i]) {
				body = append(body, lines[i])
				i++
			}
			hunks = append(hunks, patchHunk{kind: patchUpdate, path: path, moveTo: moveTo, lines: body})
		default:
			return nil, fmt.Errorf("unknown patch hunk marker %q", lines[i])
		}
	}
	if len(hunks) == 0 {
		return nil, errors.New("patch has no hunks")
	}
	return hunks, nil
}

func isHunkMarker(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "*** Add File: ") ||
		strings.HasPrefix(line, "*** Delete File: ") ||
		strings.HasPrefix(line, "*** Update File: ")
}

func applyHunk(roots []string, h patchHunk) error {
	path, err := resolvePatchPath(roots, h.path)
	if err != nil {
		return err
	}

	switch h.kind {
	case patchAdd:
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("file already exists: %s", h.path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.Join(h.lines, "\n")+"\n"), 0o644)
	case patchDelete:
		return os.Remove(path)
	case patchUpdate:
		return applyUpdateHunk(roots, path, h)
	default:
		return errors.New("unknown hunk kind")
	}
}

func applyUpdateHunk(roots []string, path string, h patchHunk) error {
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines, finalNewline := splitContent(string(originalBytes))
	chunks, err := parseUpdateChunks(h.lines)
	if err != nil {
		return err
	}
	for _, chunk := range chunks {
		idx := findLines(lines, chunk.old)
		if idx < 0 {
			return fmt.Errorf("could not find update block in %s", h.path)
		}
		replaced := make([]string, 0, len(lines)-len(chunk.old)+len(chunk.new))
		replaced = append(replaced, lines[:idx]...)
		replaced = append(replaced, chunk.new...)
		replaced = append(replaced, lines[idx+len(chunk.old):]...)
		lines = replaced
	}

	out := joinContent(lines, finalNewline)
	writePath := path
	if h.moveTo != "" {
		writePath, err = resolvePatchPath(roots, h.moveTo)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(writePath, []byte(out), 0o644); err != nil {
		return err
	}
	if h.moveTo != "" {
		return os.Remove(path)
	}
	return nil
}

type updateChunk struct {
	old []string
	new []string
}

func parseUpdateChunks(lines []string) ([]updateChunk, error) {
	var chunks []updateChunk
	current := updateChunk{}

	flush := func() {
		if len(current.old) > 0 || len(current.new) > 0 {
			chunks = append(chunks, current)
			current = updateChunk{}
		}
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "@@" || strings.HasPrefix(strings.TrimSpace(line), "@@ ") {
			flush()
			continue
		}
		if strings.TrimSpace(line) == "*** End of File" {
			continue
		}
		if line == "" {
			return nil, errors.New("empty update line must include a patch prefix")
		}
		switch line[0] {
		case ' ':
			text := line[1:]
			current.old = append(current.old, text)
			current.new = append(current.new, text)
		case '-':
			current.old = append(current.old, line[1:])
		case '+':
			current.new = append(current.new, line[1:])
		default:
			return nil, fmt.Errorf("invalid update line %q", line)
		}
	}
	flush()
	if len(chunks) == 0 {
		return []updateChunk{{}}, nil
	}
	return chunks, nil
}

// resolvePatchPath confines a patch path to one of the allowed roots; relative
// paths resolve against the first root (the session workspace).
func resolvePatchPath(roots []string, p string) (string, error) {
	if p == "" {
		return "", errors.New("patch path is empty")
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if path, err := pathsafe.Resolve(root, p); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("patch path escapes workspace: %s", p)
}

func splitContent(s string) ([]string, bool) {
	finalNewline := strings.HasSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil, finalNewline
	}
	return strings.Split(s, "\n"), finalNewline
}

func joinContent(lines []string, finalNewline bool) string {
	if len(lines) == 0 {
		if finalNewline {
			return "\n"
		}
		return ""
	}
	out := strings.Join(lines, "\n")
	if finalNewline {
		out += "\n"
	}
	return out
}

func findLines(haystack, needle []string) int {
	if len(needle) == 0 {
		return len(haystack)
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
