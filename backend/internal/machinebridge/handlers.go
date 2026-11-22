package machinebridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultListDirEntries = 200
	maxListDirEntries     = 1000
	defaultSearchResults  = 100
	maxSearchResults      = 500
	maxSearchFileBytes    = 2 << 20
	maxSnippetBytes       = 240
	defaultHTTPTimeout    = 5 * time.Minute
)

var errSearchLimit = errors.New("search limit reached")

type ExecLauncher interface {
	Start(ctx context.Context, req ExecStartRequest) (ExecStartResponse, error)
	Stop(ctx context.Context, req ExecStopRequest) error
}

type Handler struct {
	guard        *PolicyGuard
	httpClient   *http.Client
	execLauncher ExecLauncher
}

func NewHandler(guard *PolicyGuard, httpClient *http.Client, execLauncher ExecLauncher) *Handler {
	allowLoopback := guard != nil && guard.allowLoopbackTransfers
	return &Handler{
		guard:        guard,
		httpClient:   newTransferHTTPClient(httpClient, allowLoopback),
		execLauncher: execLauncher,
	}
}

func newTransferHTTPClient(base *http.Client, allowLoopback bool) *http.Client {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	if base != nil {
		copied := *base
		client = &copied
	}
	if client.Transport == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("%w: host %q resolved to no addresses", ErrInvalidRequest, host)
			}
			for _, ip := range ips {
				if isDisallowedTransferIP(ip.IP, allowLoopback) {
					return nil, fmt.Errorf("%w: resolved transfer address %s is not allowed", ErrInvalidRequest, ip.IP)
				}
			}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		}
		client.Transport = transport
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

func (h *Handler) Dispatch(ctx context.Context, caps Capabilities, method string, params json.RawMessage) (any, error) {
	switch method {
	case MethodListDir:
		var req ListDirRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.ListDir(ctx, req)
	case MethodSearch:
		var req SearchRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.Search(ctx, req)
	case MethodStageFiles:
		var req DeviceStageRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.StageFiles(ctx, caps, req)
	case MethodCommitFiles:
		var req DeviceCommitRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.CommitFiles(ctx, caps, req)
	case MethodExecStart:
		var req ExecStartRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.ExecStart(ctx, caps, req)
	case MethodExecStop:
		var req ExecStopRequest
		if err := decodeParams(params, &req); err != nil {
			return nil, err
		}
		return h.ExecStop(ctx, caps, req)
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", ErrUnsupportedCapability, method)
	}
}

func (h *Handler) ListDir(ctx context.Context, req ListDirRequest) (ListDirResponse, error) {
	if err := ctx.Err(); err != nil {
		return ListDirResponse{}, err
	}
	limit := req.MaxEntries
	if limit <= 0 {
		limit = defaultListDirEntries
	}
	if limit > maxListDirEntries {
		limit = maxListDirEntries
	}
	resolved, err := h.guard.Resolve(req.GrantID, req.Path, OpRead)
	if err != nil {
		return ListDirResponse{}, err
	}
	info, err := resolved.Root.Stat(resolved.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ListDirResponse{}, fmt.Errorf("%w: %s", ErrNotFound, req.Path)
		}
		return ListDirResponse{}, classifyRootError(err)
	}
	if !info.IsDir() {
		return ListDirResponse{}, fmt.Errorf("%w: path is not a directory", ErrInvalidRequest)
	}
	dirEntries, err := fs.ReadDir(resolved.Root.FS(), resolved.RelPath)
	if err != nil {
		return ListDirResponse{}, classifyRootError(err)
	}
	out := ListDirResponse{Entries: make([]DirEntry, 0, min(len(dirEntries), limit))}
	for _, entry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return ListDirResponse{}, err
		}
		if err := checkRelativeSegments(entry.Name()); err != nil {
			continue
		}
		childRel := joinRel(resolved.RelPath, entry.Name())
		var stat fs.FileInfo
		if entry.Type()&os.ModeSymlink != 0 {
			stat, err = resolved.Root.Stat(childRel)
			if err != nil {
				continue
			}
		} else {
			stat, err = entry.Info()
			if err != nil {
				continue
			}
		}
		if len(out.Entries) >= limit {
			out.Truncated = true
			break
		}
		size := stat.Size()
		if stat.IsDir() {
			size = 0
		}
		out.Entries = append(out.Entries, DirEntry{
			RelPath:   childRel,
			Dir:       stat.IsDir(),
			SizeBytes: size,
			MtimeMs:   stat.ModTime().UnixMilli(),
		})
	}
	return out, nil
}

func (h *Handler) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if err := ctx.Err(); err != nil {
		return SearchResponse{}, err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return SearchResponse{}, fmt.Errorf("%w: query is required", ErrInvalidRequest)
	}
	limit := req.MaxResults
	if limit <= 0 {
		limit = defaultSearchResults
	}
	if limit > maxSearchResults {
		limit = maxSearchResults
	}
	resolved, err := h.guard.Resolve(req.GrantID, req.Path, OpRead)
	if err != nil {
		return SearchResponse{}, err
	}
	startInfo, err := resolved.Root.Stat(resolved.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SearchResponse{}, fmt.Errorf("%w: %s", ErrNotFound, req.Path)
		}
		return SearchResponse{}, classifyRootError(err)
	}

	lowerQuery := strings.ToLower(query)
	out := SearchResponse{Matches: make([]SearchMatch, 0)}
	addMatch := func(match SearchMatch) error {
		if len(out.Matches) >= limit {
			out.Truncated = true
			return errSearchLimit
		}
		out.Matches = append(out.Matches, match)
		return nil
	}

	searchFile := func(relPath string, size int64) error {
		if strings.Contains(strings.ToLower(relPath), lowerQuery) {
			if err := addMatch(SearchMatch{RelPath: relPath}); err != nil {
				return err
			}
		}
		if size <= 0 || size > maxSearchFileBytes {
			return nil
		}
		f, err := resolved.Root.Open(relPath)
		if err != nil {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(f, maxSearchFileBytes+1))
		_ = f.Close()
		if err != nil {
			return nil
		}
		if len(data) > maxSearchFileBytes || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			return nil
		}
		lineNo := 0
		for rawLine := range bytes.SplitSeq(data, []byte{'\n'}) {
			if err := ctx.Err(); err != nil {
				return err
			}
			lineNo++
			line := strings.TrimRight(string(rawLine), "\r")
			if strings.Contains(strings.ToLower(line), lowerQuery) {
				if err := addMatch(SearchMatch{
					RelPath: relPath,
					Line:    lineNo,
					Snippet: truncateSnippet(strings.TrimSpace(line), maxSnippetBytes),
				}); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if !startInfo.IsDir() {
		if startInfo.Mode().IsRegular() {
			if err := searchFile(resolved.RelPath, startInfo.Size()); err != nil && !errors.Is(err, errSearchLimit) {
				return SearchResponse{}, err
			}
		}
		return out, nil
	}

	walkErr := fs.WalkDir(resolved.Root.FS(), resolved.RelPath, func(relPath string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if relPath != resolved.RelPath && shouldSkipSearchDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		return searchFile(relPath, info.Size())
	})
	if walkErr != nil && !errors.Is(walkErr, errSearchLimit) {
		return SearchResponse{}, walkErr
	}
	return out, nil
}

func (h *Handler) StageFiles(ctx context.Context, caps Capabilities, req DeviceStageRequest) (DeviceStageResponse, error) {
	if !caps.FileStaging {
		return DeviceStageResponse{}, fmt.Errorf("%w: file staging disabled", ErrUnsupportedCapability)
	}
	resolvedPaths, err := h.guard.ValidateStageRequest(req)
	if err != nil {
		return DeviceStageResponse{}, err
	}
	staged := make([]StagedFile, 0, len(req.Files))
	for i, target := range req.Files {
		if err := ctx.Err(); err != nil {
			return DeviceStageResponse{}, err
		}
		item, err := h.stageOne(ctx, caps.MaxStageBytes, resolvedPaths[i], target)
		if err != nil {
			return DeviceStageResponse{}, err
		}
		staged = append(staged, item)
	}
	return DeviceStageResponse{Files: staged}, nil
}

func (h *Handler) stageOne(ctx context.Context, maxBytes int64, resolved ResolvedPath, target StageFileTarget) (StagedFile, error) {
	file, err := resolved.Root.Open(resolved.RelPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StagedFile{}, fmt.Errorf("%w: %s", ErrNotFound, resolved.RelPath)
		}
		return StagedFile{}, classifyRootError(err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return StagedFile{}, err
	}
	if !info.Mode().IsRegular() {
		return StagedFile{}, fmt.Errorf("%w: %s is not a regular file", ErrInvalidRequest, resolved.RelPath)
	}
	size := info.Size()
	if maxBytes > 0 && size > maxBytes {
		return StagedFile{}, fmt.Errorf("%w: %s size %d exceeds max_stage_bytes %d", ErrInvalidRequest, resolved.RelPath, size, maxBytes)
	}

	contentType := resolveUploadContentType(target.ContentType, target.UploadHeaders)
	if contentType == "" {
		contentType = detectContentType(file, size)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return StagedFile{}, err
		}
	}

	hasher := sha256.New()
	var counter byteCounter
	var body io.Reader
	if size == 0 {
		body = http.NoBody
	} else {
		body = io.TeeReader(io.LimitReader(file, size), io.MultiWriter(hasher, &counter))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimSpace(target.UploadURL), body)
	if err != nil {
		return StagedFile{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	httpReq.ContentLength = size
	if err := applyTransferHeaders(httpReq, target.UploadHeaders); err != nil {
		return StagedFile{}, err
	}
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return StagedFile{}, fmt.Errorf("upload %s: %w", resolved.RelPath, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return StagedFile{}, fmt.Errorf("upload %s: unexpected HTTP status %d", resolved.RelPath, resp.StatusCode)
	}
	if counter.n != size {
		return StagedFile{}, fmt.Errorf("%w: %s changed size during upload", ErrConflict, resolved.RelPath)
	}

	return StagedFile{
		RelPath:     resolved.RelPath,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes:   size,
		MtimeMs:     info.ModTime().UnixMilli(),
		ContentType: httpReq.Header.Get("Content-Type"),
	}, nil
}

type stagedCommitFile struct {
	resolved  ResolvedPath
	entry     CommitFileEntry
	tmpRel    string
	sha256Hex string
	sizeBytes int64
	committed bool
}

func (h *Handler) CommitFiles(ctx context.Context, caps Capabilities, req DeviceCommitRequest) (DeviceCommitResponse, error) {
	if !caps.FileStaging {
		return DeviceCommitResponse{}, fmt.Errorf("%w: file staging disabled", ErrUnsupportedCapability)
	}
	resolvedPaths, err := h.guard.ValidateCommitRequest(req)
	if err != nil {
		return DeviceCommitResponse{}, err
	}
	// Phase 1: check preconditions and target types before downloading anything.
	for i, entry := range req.Files {
		resolved := resolvedPaths[i]
		if entry.Delete {
			if err := verifyDeletableTarget(resolved.Root, resolved.RelPath); err != nil {
				return DeviceCommitResponse{}, err
			}
			if err := checkRootPrecondition(resolved.Root, resolved.RelPath, entry.ExpectedMtimeMs, entry.ExpectedSHA256, true); err != nil {
				return DeviceCommitResponse{}, err
			}
			continue
		}
		if err := checkRootPrecondition(resolved.Root, resolved.RelPath, entry.ExpectedMtimeMs, entry.ExpectedSHA256, false); err != nil {
			return DeviceCommitResponse{}, err
		}
	}

	// Phase 2: stage all downloads into temp files before mutating any target file.
	staged := make([]stagedCommitFile, len(req.Files))
	defer func() {
		for _, item := range staged {
			if item.tmpRel != "" && !item.committed {
				_ = item.resolved.Root.Remove(item.tmpRel)
			}
		}
	}()
	for i, entry := range req.Files {
		if err := ctx.Err(); err != nil {
			return DeviceCommitResponse{}, err
		}
		staged[i] = stagedCommitFile{resolved: resolvedPaths[i], entry: entry}
		if entry.Delete {
			continue
		}
		tmpRel, digest, written, err := h.downloadCommitTemp(ctx, caps.MaxStageBytes, resolvedPaths[i], entry)
		if err != nil {
			return DeviceCommitResponse{}, err
		}
		staged[i].tmpRel = tmpRel
		staged[i].sha256Hex = digest
		staged[i].sizeBytes = written
	}

	// Phase 3: re-check all preconditions across the batch before applying mutations.
	for _, item := range staged {
		if item.entry.Delete {
			if err := verifyDeletableTarget(item.resolved.Root, item.resolved.RelPath); err != nil {
				return DeviceCommitResponse{}, err
			}
		}
		if err := checkRootPrecondition(item.resolved.Root, item.resolved.RelPath, item.entry.ExpectedMtimeMs, item.entry.ExpectedSHA256, item.entry.Delete); err != nil {
			return DeviceCommitResponse{}, err
		}
	}

	// Phase 4: apply renames and deletes through os.Root.
	committed := make([]CommittedFile, 0, len(staged))
	for i := range staged {
		item := &staged[i]
		if item.entry.Delete {
			if err := verifyDeletableTarget(item.resolved.Root, item.resolved.RelPath); err != nil {
				return DeviceCommitResponse{}, err
			}
			if err := checkRootPrecondition(item.resolved.Root, item.resolved.RelPath, item.entry.ExpectedMtimeMs, item.entry.ExpectedSHA256, true); err != nil {
				return DeviceCommitResponse{}, err
			}
			if err := item.resolved.Root.Remove(item.resolved.RelPath); err != nil {
				return DeviceCommitResponse{}, classifyRootError(err)
			}
			committed = append(committed, CommittedFile{RelPath: item.resolved.RelPath, Deleted: true})
			continue
		}
		if err := checkRootPrecondition(item.resolved.Root, item.resolved.RelPath, item.entry.ExpectedMtimeMs, item.entry.ExpectedSHA256, false); err != nil {
			return DeviceCommitResponse{}, err
		}
		if err := item.resolved.Root.Rename(item.tmpRel, item.resolved.RelPath); err != nil {
			return DeviceCommitResponse{}, classifyRootError(err)
		}
		item.committed = true
		info, err := item.resolved.Root.Stat(item.resolved.RelPath)
		if err != nil {
			return DeviceCommitResponse{}, classifyRootError(err)
		}
		committed = append(committed, CommittedFile{
			RelPath:   item.resolved.RelPath,
			SHA256:    item.sha256Hex,
			SizeBytes: item.sizeBytes,
			MtimeMs:   info.ModTime().UnixMilli(),
		})
	}
	return DeviceCommitResponse{Files: committed}, nil
}

func verifyDeletableTarget(root *os.Root, rel string) error {
	info, err := root.Lstat(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, rel)
		}
		return classifyRootError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: cannot delete symlink %s", ErrSymlinkEscape, rel)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: cannot delete directory %s", ErrInvalidRequest, rel)
	}
	return nil
}

func (h *Handler) downloadCommitTemp(ctx context.Context, maxBytes int64, resolved ResolvedPath, entry CommitFileEntry) (string, string, int64, error) {
	perm := fs.FileMode(0o644)
	if existing, err := resolved.Root.Lstat(resolved.RelPath); err == nil {
		if existing.IsDir() {
			return "", "", 0, fmt.Errorf("%w: cannot overwrite directory %s", ErrInvalidRequest, resolved.RelPath)
		}
		if existing.Mode()&os.ModeSymlink == 0 {
			perm = existing.Mode().Perm()
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", 0, classifyRootError(err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(entry.DownloadURL), nil)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if err := applyTransferHeaders(httpReq, entry.DownloadHeaders); err != nil {
		return "", "", 0, err
	}

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return "", "", 0, fmt.Errorf("download %s: %w", resolved.RelPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", "", 0, fmt.Errorf("download %s: unexpected HTTP status %d", resolved.RelPath, resp.StatusCode)
	}

	parentRel := path.Dir(resolved.RelPath)
	if parentRel != "." && parentRel != "" {
		if err := resolved.Root.MkdirAll(parentRel, 0o755); err != nil {
			return "", "", 0, classifyRootError(err)
		}
	}
	tmp, tmpRel, err := createTempInRoot(resolved.Root, parentRel, ".jaz-commit-")
	if err != nil {
		return "", "", 0, classifyRootError(err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = resolved.Root.Remove(tmpRel)
		}
	}()

	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), reader)
	if err != nil {
		_ = tmp.Close()
		return "", "", 0, err
	}
	if maxBytes > 0 && written > maxBytes {
		_ = tmp.Close()
		return "", "", 0, fmt.Errorf("%w: %s download exceeds max_stage_bytes %d", ErrInvalidRequest, resolved.RelPath, maxBytes)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return "", "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", "", 0, err
	}
	cleanup = false
	return tmpRel, hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func createTempInRoot(root *os.Root, parentRel, prefix string) (*os.File, string, error) {
	var raw [8]byte
	for range 16 {
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, "", err
		}
		name := prefix + hex.EncodeToString(raw[:])
		rel := joinRel(parentRel, name)
		f, err := root.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, rel, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("create temp file in %q: exhausted retries", parentRel)
}

func (h *Handler) ExecStart(ctx context.Context, caps Capabilities, req ExecStartRequest) (ExecStartResponse, error) {
	if !caps.LocalExec || h.execLauncher == nil {
		return ExecStartResponse{}, fmt.Errorf("%w: local exec disabled", ErrUnsupportedCapability)
	}
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" {
		return ExecStartResponse{}, fmt.Errorf("%w: grant_id and run_id are required", ErrInvalidRequest)
	}
	if _, err := h.guard.GrantRoot(req.GrantID, OpWrite); err != nil {
		return ExecStartResponse{}, err
	}
	return h.execLauncher.Start(ctx, req)
}

func (h *Handler) ExecStop(ctx context.Context, caps Capabilities, req ExecStopRequest) (ExecStopResponse, error) {
	if !caps.LocalExec || h.execLauncher == nil {
		return ExecStopResponse{}, fmt.Errorf("%w: local exec disabled", ErrUnsupportedCapability)
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return ExecStopResponse{}, fmt.Errorf("%w: session_id is required", ErrInvalidRequest)
	}
	if err := h.execLauncher.Stop(ctx, req); err != nil {
		return ExecStopResponse{}, err
	}
	return ExecStopResponse{Stopped: true}, nil
}

func decodeParams(raw json.RawMessage, out any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%w: params required", ErrInvalidRequest)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: decode params: %v", ErrInvalidRequest, err)
	}
	return nil
}

func joinRel(parent, name string) string {
	if parent == "" || parent == "." {
		return name
	}
	return parent + "/" + name
}

func shouldSkipSearchDir(name string) bool {
	return name == ".git" || name == "node_modules"
}

func truncateSnippet(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func resolveUploadContentType(explicit string, headers map[string]string) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	for k, v := range headers {
		if strings.EqualFold(strings.TrimSpace(k), "Content-Type") {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func detectContentType(file *os.File, size int64) string {
	if size == 0 {
		return "application/octet-stream"
	}
	buf := make([]byte, min(int64(512), size))
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func applyTransferHeaders(req *http.Request, headers map[string]string) error {
	for k, v := range headers {
		key := strings.TrimSpace(k)
		if key == "" || strings.EqualFold(key, "Content-Length") {
			continue
		}
		if strings.EqualFold(key, "Host") {
			return fmt.Errorf("%w: Host header override is not allowed", ErrInvalidRequest)
		}
		req.Header.Set(key, v)
	}
	return nil
}

type byteCounter struct {
	n int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
