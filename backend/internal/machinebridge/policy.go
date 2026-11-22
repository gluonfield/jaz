package machinebridge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

const defaultTransferHost = "storage.googleapis.com"

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

type PolicyOption func(*PolicyGuard)

// WithAllowLoopbackTransfers permits HTTP/HTTPS transfers to loopback addresses.
// Intended for local tests only; production defaults to rejecting loopback and private IPs.
func WithAllowLoopbackTransfers() PolicyOption {
	return func(g *PolicyGuard) {
		g.allowLoopbackTransfers = true
	}
}

// WithAllowedTransferHosts pins permitted remote transfer hostnames (e.g.
// storage.googleapis.com and the CloudURL host) so a compromised token cannot
// exfiltrate local files to an arbitrary third-party HTTPS server.
func WithAllowedTransferHosts(hosts ...string) PolicyOption {
	return func(g *PolicyGuard) {
		if g.allowedTransferHosts == nil {
			g.allowedTransferHosts = make(map[string]struct{}, len(hosts))
		}
		for _, h := range hosts {
			if norm := normalizeTransferHost(h); norm != "" {
				g.allowedTransferHosts[norm] = struct{}{}
			}
		}
	}
}

type grantEntry struct {
	grant    FolderGrant
	rawRoot  string
	realRoot string
	root     *os.Root
}

type ResolvedPath struct {
	GrantID  string
	RelPath  string
	AbsPath  string
	RealPath string
	Root     *os.Root
}

// PolicyGuard enforces folder-grant access modes (ro/rw/rwd), lexical path
// validation, and kernel-enforced os.Root confinement against symlink escapes.
type PolicyGuard struct {
	grants                 map[string]grantEntry
	allowLoopbackTransfers bool
	allowedTransferHosts   map[string]struct{}
}

func NewPolicyGuard(grants []FolderGrant, opts ...PolicyOption) (*PolicyGuard, error) {
	entries := make(map[string]grantEntry, len(grants))
	cleanup := func() {
		for _, entry := range entries {
			_ = entry.root.Close()
		}
	}
	for _, grant := range grants {
		id := strings.TrimSpace(grant.ID)
		if id == "" {
			cleanup()
			return nil, fmt.Errorf("%w: grant id is required", ErrInvalidRequest)
		}
		if _, exists := entries[id]; exists {
			cleanup()
			return nil, fmt.Errorf("%w: duplicate grant id %q", ErrInvalidRequest, id)
		}
		if !grant.Mode.Valid() {
			cleanup()
			return nil, fmt.Errorf("%w: invalid access mode %q", ErrInvalidRequest, grant.Mode)
		}
		root := strings.TrimSpace(grant.RootPath)
		if root == "" || strings.ContainsRune(root, '\x00') || !filepath.IsAbs(root) {
			cleanup()
			return nil, fmt.Errorf("%w: grant root must be an absolute path", ErrInvalidRequest)
		}
		rawRoot := filepath.Clean(root)
		realRoot, err := filepath.EvalSymlinks(rawRoot)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("resolve grant %q root: %w", id, err)
		}
		info, err := os.Stat(realRoot)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("stat grant %q root: %w", id, err)
		}
		if !info.IsDir() {
			cleanup()
			return nil, fmt.Errorf("%w: grant %q root is not a directory", ErrInvalidRequest, id)
		}
		rootHandle, err := os.OpenRoot(realRoot)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("open grant %q root: %w", id, err)
		}
		grant.ID = id
		grant.RootPath = realRoot
		entries[id] = grantEntry{
			grant:    grant,
			rawRoot:  rawRoot,
			realRoot: realRoot,
			root:     rootHandle,
		}
	}
	guard := &PolicyGuard{grants: entries}
	for _, opt := range opts {
		if opt != nil {
			opt(guard)
		}
	}
	if len(guard.allowedTransferHosts) == 0 {
		guard.allowedTransferHosts = map[string]struct{}{defaultTransferHost: {}}
	}
	return guard, nil
}

func (g *PolicyGuard) Close() error {
	var errs []error
	for _, entry := range g.grants {
		if entry.root != nil {
			errs = append(errs, entry.root.Close())
		}
	}
	return errors.Join(errs...)
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

func (g *PolicyGuard) GrantRoot(grantID string, op Operation) (string, error) {
	entry, ok := g.grants[grantID]
	if !ok {
		return "", fmt.Errorf("%w: unknown grant %q", ErrAccessDenied, grantID)
	}
	if !entry.grant.Mode.Allows(op) {
		return "", fmt.Errorf("%w: grant %q (%s) does not permit %s", ErrAccessDenied, grantID, entry.grant.Mode, op)
	}
	return entry.realRoot, nil
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
	realPath, err := verifyRootConfinement(entry.root, entry.realRoot, rel, op)
	if err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{
		GrantID:  entry.grant.ID,
		RelPath:  rel,
		AbsPath:  abs,
		RealPath: realPath,
		Root:     entry.root,
	}, nil
}

func (g *PolicyGuard) ValidateStageRequest(req DeviceStageRequest) ([]ResolvedPath, error) {
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" || len(req.Files) == 0 {
		return nil, fmt.Errorf("%w: grant_id, run_id, and files are required", ErrInvalidRequest)
	}
	resolved := make([]ResolvedPath, 0, len(req.Files))
	for _, file := range req.Files {
		if err := validateTransferURL(file.UploadURL, g.allowLoopbackTransfers, g.allowedTransferHosts); err != nil {
			return nil, err
		}
		if err := validateTransferHeaders(file.UploadHeaders); err != nil {
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
		} else {
			if err := validateTransferURL(file.DownloadURL, g.allowLoopbackTransfers, g.allowedTransferHosts); err != nil {
				return nil, err
			}
			if err := validateTransferHeaders(file.DownloadHeaders); err != nil {
				return nil, err
			}
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
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		got, err := readerSHA256(f)
		if err != nil {
			return err
		}
		if got != expectedSHA256 {
			return fmt.Errorf("%w: sha256 mismatch", ErrConflict)
		}
	}
	return nil
}

func checkRootPrecondition(root *os.Root, rel string, expectedMtimeMs *int64, expectedSHA256 string, noFollow bool) error {
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expectedMtimeMs == nil && expectedSHA256 == "" {
		return nil
	}
	var info fs.FileInfo
	var err error
	if noFollow {
		info, err = root.Lstat(rel)
	} else {
		info, err = root.Stat(rel)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: file missing on device", ErrConflict)
		}
		return classifyRootError(err)
	}
	if noFollow && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: cannot follow symlink for delete precondition", ErrSymlinkEscape)
	}
	if expectedMtimeMs != nil && info.ModTime().UnixMilli() != *expectedMtimeMs {
		return fmt.Errorf("%w: mtime changed (%d != %d)", ErrConflict, info.ModTime().UnixMilli(), *expectedMtimeMs)
	}
	if expectedSHA256 != "" {
		f, err := root.Open(rel)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: file missing on device", ErrConflict)
			}
			return classifyRootError(err)
		}
		defer f.Close()
		got, err := readerSHA256(f)
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
	return cleanRel, filepath.Join(entry.realRoot, filepath.FromSlash(cleanRel)), nil
}

func verifyRootConfinement(root *os.Root, realRoot, cleanRel string, op Operation) (string, error) {
	if cleanRel == "." || cleanRel == "" {
		return realRoot, nil
	}
	candidate := filepath.Join(realRoot, filepath.FromSlash(cleanRel))
	if op == OpDelete {
		info, err := root.Lstat(cleanRel)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("%w: symlink delete rejected", ErrSymlinkEscape)
			}
			return candidate, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		}
		return "", classifyRootError(err)
	}
	if _, err := root.Stat(cleanRel); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		}
		return "", classifyRootError(err)
	}
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		return resolved, nil
	}
	return candidate, nil
}

func classifyRootError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ELOOP) ||
		strings.Contains(err.Error(), "escapes from parent") ||
		strings.Contains(err.Error(), "too many levels of symbolic links") ||
		strings.Contains(err.Error(), "too many links") {
		return fmt.Errorf("%w: %v", ErrSymlinkEscape, err)
	}
	return err
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

func validateTransferURL(raw string, allowLoopback bool, allowedHosts map[string]struct{}) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%w: valid transfer URL is required", ErrInvalidRequest)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: transfer URL host is required", ErrInvalidRequest)
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !allowLoopback || !isLoopbackHost(host) {
			return fmt.Errorf("%w: transfer URL must use https", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: transfer URL must use https", ErrInvalidRequest)
	}
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		if !allowLoopback {
			return fmt.Errorf("%w: loopback transfer URL is not allowed", ErrInvalidRequest)
		}
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedTransferIP(ip, allowLoopback) {
			return fmt.Errorf("%w: private or loopback transfer address %q is not allowed", ErrInvalidRequest, host)
		}
		if allowLoopback && ip.IsLoopback() {
			return nil
		}
	}
	normHost := normalizeTransferHost(host)
	if _, ok := allowedHosts[normHost]; !ok {
		return fmt.Errorf("%w: transfer host %q is not allowed", ErrInvalidRequest, host)
	}
	return nil
}

func normalizeTransferHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if parsed, err := url.Parse(s); err == nil {
			s = parsed.Hostname()
		}
	} else if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.Trim(s, "[]")
	s = strings.TrimSuffix(s, ".")
	return strings.ToLower(strings.TrimSpace(s))
}

func validateTransferHeaders(headers map[string]string) error {
	for k := range headers {
		if strings.EqualFold(strings.TrimSpace(k), "Host") {
			return fmt.Errorf("%w: Host header override is not allowed", ErrInvalidRequest)
		}
	}
	return nil
}

func isDisallowedTransferIP(ip net.IP, allowLoopback bool) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return !allowLoopback
	}
	return addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsUnspecified() ||
		addr.IsMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		cgnatPrefix.Contains(addr)
}

func isLoopbackHost(host string) bool {
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func readerSHA256(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func relWithin(root, target string) (string, bool) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
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
