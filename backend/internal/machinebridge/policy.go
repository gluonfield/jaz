package machinebridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
)

const maxSymlinkDepth = 40

type grantEntry struct {
	grant    FolderGrant
	rawRoot  string
	realRoot string
}

type ResolvedPath struct {
	GrantID  string
	RelPath  string
	AbsPath  string
	RealPath string
}

// PolicyGuard enforces folder-grant access modes (ro/rw/rwd), lexical path
// confinement across macOS and Windows rules, and realpath symlink escape checks.
type PolicyGuard struct {
	grants map[string]grantEntry
}

func NewPolicyGuard(grants []FolderGrant) (*PolicyGuard, error) {
	entries := make(map[string]grantEntry, len(grants))
	for _, grant := range grants {
		id := strings.TrimSpace(grant.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: grant id is required", ErrInvalidRequest)
		}
		if _, exists := entries[id]; exists {
			return nil, fmt.Errorf("%w: duplicate grant id %q", ErrInvalidRequest, id)
		}
		if !grant.Mode.Valid() {
			return nil, fmt.Errorf("%w: invalid access mode %q", ErrInvalidRequest, grant.Mode)
		}
		root := strings.TrimSpace(grant.RootPath)
		if root == "" || strings.ContainsRune(root, '\x00') || !filepath.IsAbs(root) {
			return nil, fmt.Errorf("%w: grant root must be an absolute path", ErrInvalidRequest)
		}
		rawRoot := filepath.Clean(root)
		realRoot, err := filepath.EvalSymlinks(rawRoot)
		if err != nil {
			return nil, fmt.Errorf("resolve grant %q root: %w", id, err)
		}
		info, err := os.Stat(realRoot)
		if err != nil {
			return nil, fmt.Errorf("stat grant %q root: %w", id, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: grant %q root is not a directory", ErrInvalidRequest, id)
		}
		grant.ID = id
		grant.RootPath = realRoot
		entries[id] = grantEntry{
			grant:    grant,
			rawRoot:  rawRoot,
			realRoot: realRoot,
		}
	}
	return &PolicyGuard{grants: entries}, nil
}

func (g *PolicyGuard) Grants() []FolderGrant {
	out := make([]FolderGrant, 0, len(g.grants))
	for _, entry := range g.grants {
		out = append(out, entry.grant)
	}
	slices.SortFunc(out, func(a, b FolderGrant) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// ParseGrantSpec parses "<id>=<mode>:<path>" into a FolderGrant.
// Splitting on the first ":" preserves Windows drive letters (e.g. "work=rw:C:\Proj").
func ParseGrantSpec(spec string) (FolderGrant, error) {
	id, rest, ok := strings.Cut(strings.TrimSpace(spec), "=")
	if !ok {
		return FolderGrant{}, fmt.Errorf("%w: grant must have form id=mode:path", ErrInvalidRequest)
	}
	modeRaw, rootPath, ok := strings.Cut(rest, ":")
	if !ok {
		return FolderGrant{}, fmt.Errorf("%w: grant must have form id=mode:path", ErrInvalidRequest)
	}
	id = strings.TrimSpace(id)
	mode := AccessMode(strings.TrimSpace(modeRaw))
	rootPath = strings.TrimSpace(rootPath)
	if id == "" || !mode.Valid() || rootPath == "" {
		return FolderGrant{}, fmt.Errorf("%w: invalid grant %q (expected id=ro|rw|rwd:path)", ErrInvalidRequest, spec)
	}
	if !filepath.IsAbs(rootPath) && !isWindowsAbs(rootPath) {
		abs, err := filepath.Abs(rootPath)
		if err != nil {
			return FolderGrant{}, fmt.Errorf("%w: resolve grant path %q: %v", ErrInvalidRequest, rootPath, err)
		}
		rootPath = abs
	}
	return FolderGrant{
		ID:       id,
		Mode:     mode,
		RootPath: rootPath,
	}, nil
}

func (g *PolicyGuard) Resolve(grantID, target string, op Operation) (ResolvedPath, error) {
	entry, ok := g.grants[grantID]
	if !ok {
		return ResolvedPath{}, fmt.Errorf("%w: unknown grant %q", ErrAccessDenied, grantID)
	}
	if !entry.grant.Mode.Allows(op) {
		return ResolvedPath{}, fmt.Errorf("%w: grant %q (%s) does not permit %s", ErrAccessDenied, grantID, entry.grant.Mode, op)
	}
	rel, abs, err := lexicalConfine(entry, target)
	if err != nil {
		return ResolvedPath{}, err
	}
	if rel == "." && op != OpRead {
		return ResolvedPath{}, fmt.Errorf("%w: cannot %s grant root directory", ErrAccessDenied, op)
	}
	realPath, err := resolveConfinedRealPath(entry.realRoot, rel)
	if err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{
		GrantID:  entry.grant.ID,
		RelPath:  rel,
		AbsPath:  abs,
		RealPath: realPath,
	}, nil
}

func (g *PolicyGuard) ValidateStageRequest(req DeviceStageRequest) ([]ResolvedPath, error) {
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" || len(req.Files) == 0 {
		return nil, fmt.Errorf("%w: grant_id, run_id, and files are required", ErrInvalidRequest)
	}
	resolved := make([]ResolvedPath, 0, len(req.Files))
	for _, file := range req.Files {
		if err := validateTransferURL(file.UploadURL); err != nil {
			return nil, err
		}
		path, err := g.Resolve(req.GrantID, file.RelPath, OpRead)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

func (g *PolicyGuard) ValidateCommitRequest(req DeviceCommitRequest) ([]ResolvedPath, error) {
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" || len(req.Files) == 0 {
		return nil, fmt.Errorf("%w: grant_id, run_id, and files are required", ErrInvalidRequest)
	}
	resolved := make([]ResolvedPath, 0, len(req.Files))
	for _, file := range req.Files {
		op := OpWrite
		if file.Delete {
			op = OpDelete
		} else if err := validateTransferURL(file.DownloadURL); err != nil {
			return nil, err
		}
		path, err := g.Resolve(req.GrantID, file.RelPath, op)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

// CheckCommitPrecondition verifies expected mtime/sha256 before writing back
// staged outputs so a local edit made while the cloud turn ran is not clobbered.
func CheckCommitPrecondition(path string, expectedMtimeMs *int64, expectedSHA256 string) error {
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expectedMtimeMs == nil && expectedSHA256 == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: file missing on device", ErrConflict)
		}
		return err
	}
	if expectedMtimeMs != nil && info.ModTime().UnixMilli() != *expectedMtimeMs {
		return fmt.Errorf("%w: mtime changed (%d != %d)", ErrConflict, info.ModTime().UnixMilli(), *expectedMtimeMs)
	}
	if expectedSHA256 != "" {
		got, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if got != expectedSHA256 {
			return fmt.Errorf("%w: sha256 mismatch", ErrConflict)
		}
	}
	return nil
}

func lexicalConfine(entry grantEntry, raw string) (rel string, abs string, err error) {
	if strings.ContainsRune(raw, '\x00') {
		return "", "", fmt.Errorf("%w: NUL byte in path", ErrPathOutsideGrant)
	}
	if hasWindowsDevicePrefix(raw) {
		return "", "", fmt.Errorf("%w: device or UNC path rejected", ErrPathOutsideGrant)
	}
	if raw == "" || raw == "." {
		return ".", entry.realRoot, nil
	}
	if filepath.IsAbs(raw) || isWindowsAbs(raw) {
		if !filepath.IsAbs(raw) {
			return "", "", fmt.Errorf("%w: foreign absolute path", ErrPathOutsideGrant)
		}
		clean := filepath.Clean(raw)
		if relPath, ok := relWithin(entry.realRoot, clean); ok {
			if err := checkRelativeSegments(relPath); err != nil {
				return "", "", err
			}
			return relPath, clean, nil
		}
		if relPath, ok := relWithin(entry.rawRoot, clean); ok {
			if err := checkRelativeSegments(relPath); err != nil {
				return "", "", err
			}
			return relPath, filepath.Join(entry.realRoot, filepath.FromSlash(relPath)), nil
		}
		return "", "", fmt.Errorf("%w: %s", ErrPathOutsideGrant, raw)
	}
	if err := validateRelativeInput(raw); err != nil {
		return "", "", err
	}
	normalized := strings.ReplaceAll(raw, "\\", "/")
	cleanRel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(normalized)))
	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") || strings.HasPrefix(cleanRel, "/") {
		return "", "", fmt.Errorf("%w: %s", ErrPathOutsideGrant, raw)
	}
	if err := checkRelativeSegments(cleanRel); err != nil {
		return "", "", err
	}
	joined := filepath.Join(entry.realRoot, filepath.FromSlash(cleanRel))
	if !withinOSPath(entry.realRoot, joined) {
		return "", "", fmt.Errorf("%w: %s", ErrPathOutsideGrant, raw)
	}
	return cleanRel, joined, nil
}

func validateRelativeInput(raw string) error {
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") {
		return fmt.Errorf("%w: leading separator", ErrPathOutsideGrant)
	}
	if len(raw) >= 2 && isASCIILetter(raw[0]) && raw[1] == ':' {
		return fmt.Errorf("%w: drive prefix in relative path", ErrPathOutsideGrant)
	}
	for segment := range strings.SplitSeq(strings.ReplaceAll(raw, "\\", "/"), "/") {
		if segment == ".." {
			return fmt.Errorf("%w: traversal segment", ErrPathOutsideGrant)
		}
		if strings.ContainsRune(segment, ':') {
			return fmt.Errorf("%w: colon not allowed in relative path segment", ErrPathOutsideGrant)
		}
	}
	return nil
}

func checkRelativeSegments(cleanRel string) error {
	if cleanRel == "." || cleanRel == "" {
		return nil
	}
	for segment := range strings.SplitSeq(cleanRel, "/") {
		if segment == ".." || strings.ContainsRune(segment, ':') || isWindowsReservedName(segment) {
			return fmt.Errorf("%w: disallowed path segment %q", ErrPathOutsideGrant, segment)
		}
	}
	return nil
}

// resolveConfinedRealPath walks every path component from realRoot, resolving
// both existing and dangling symlinks so neither reads nor new-file writes can
// escape realRoot through a symlink or symlink chain.
func resolveConfinedRealPath(realRoot, cleanRel string) (string, error) {
	if cleanRel == "." || cleanRel == "" {
		return realRoot, nil
	}
	parts := strings.Split(cleanRel, "/")
	return walkConfined(realRoot, realRoot, parts, 0)
}

func walkConfined(realRoot, current string, parts []string, depth int) (string, error) {
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("%w: traversal segment", ErrSymlinkEscape)
		}
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) {
			remaining := append([]string{part}, parts[i+1:]...)
			candidate := filepath.Join(append([]string{current}, remaining...)...)
			if !withinOSPath(realRoot, candidate) {
				return "", fmt.Errorf("%w: %s", ErrSymlinkEscape, candidate)
			}
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if !withinOSPath(realRoot, next) {
				return "", fmt.Errorf("%w: %s", ErrSymlinkEscape, next)
			}
			current = next
			continue
		}
		if depth >= maxSymlinkDepth {
			return "", fmt.Errorf("%w: too many symlink levels", ErrSymlinkEscape)
		}
		linkTarget, err := os.Readlink(next)
		if err != nil {
			return "", err
		}
		resolvedTarget := linkTarget
		if !filepath.IsAbs(resolvedTarget) {
			resolvedTarget = filepath.Join(current, resolvedTarget)
		}
		resolvedTarget = filepath.Clean(resolvedTarget)
		resolvedTarget, err = resolveDeepestAncestor(resolvedTarget)
		if err != nil {
			return "", err
		}
		if !withinOSPath(realRoot, resolvedTarget) {
			return "", fmt.Errorf("%w: symlink points outside grant", ErrSymlinkEscape)
		}
		relAfterLink, ok := relWithin(realRoot, resolvedTarget)
		if !ok {
			return "", fmt.Errorf("%w: symlink points outside grant", ErrSymlinkEscape)
		}
		var nextParts []string
		if relAfterLink != "." && relAfterLink != "" {
			nextParts = append(nextParts, strings.Split(relAfterLink, "/")...)
		}
		nextParts = append(nextParts, parts[i+1:]...)
		return walkConfined(realRoot, realRoot, nextParts, depth+1)
	}
	if !withinOSPath(realRoot, current) {
		return "", fmt.Errorf("%w: %s", ErrSymlinkEscape, current)
	}
	return current, nil
}

// resolveDeepestAncestor canonicalizes the deepest existing prefix of target so
// symlink chains inside macOS /var -> /private/var or nested symlinks compare
// cleanly against realRoot even when the final target does not exist yet.
func resolveDeepestAncestor(target string) (string, error) {
	var tail []string
	probe := target
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			if errors.Is(err, syscall.ELOOP) || strings.Contains(err.Error(), "too many links") {
				return "", fmt.Errorf("%w: %v", ErrSymlinkEscape, err)
			}
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return target, nil
		}
		tail = append(tail, filepath.Base(probe))
		probe = parent
	}
}

func validateTransferURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: valid transfer URL is required", ErrInvalidRequest)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("%w: transfer URL must use https", ErrInvalidRequest)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func relWithin(root, target string) (string, bool) {
	if !withinOSPath(root, target) {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func withinOSPath(root, target string) bool {
	return withinPath(root, target, runtime.GOOS == "windows", filepath.Separator)
}

// withinPath checks lexical containment with explicit case-sensitivity and
// separator rules so Windows drive/case behavior is unit-tested on every OS.
func withinPath(root, target string, caseInsensitive bool, sep byte) bool {
	sepStr := string(sep)
	norm := func(s string) string {
		if sep == '\\' {
			s = strings.ReplaceAll(s, "/", `\`)
		}
		s = strings.TrimRight(s, sepStr)
		if s == "" && strings.HasPrefix(root, sepStr) {
			return sepStr
		}
		if caseInsensitive {
			return strings.ToLower(s)
		}
		return s
	}
	r := norm(root)
	t := norm(target)
	if r == "" || t == "" {
		return false
	}
	if r == sepStr {
		return strings.HasPrefix(t, sepStr)
	}
	if t == r {
		return true
	}
	return strings.HasPrefix(t, r+sepStr)
}

func hasWindowsDevicePrefix(s string) bool {
	return strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, `//`)
}

func isWindowsAbs(s string) bool {
	return len(s) >= 3 && isASCIILetter(s[0]) && s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isWindowsReservedName(segment string) bool {
	base, _, _ := strings.Cut(strings.TrimRight(segment, " ."), ".")
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
