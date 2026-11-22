package machinebridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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
	Start(ctx context.Context, req ExecStartRequest, grantRoot string) (ExecStartResponse, error)
	Stop(ctx context.Context, req ExecStopRequest) error
}

type Handler struct {
	guard        *PolicyGuard
	httpClient   *http.Client
	execLauncher ExecLauncher
}

func NewHandler(guard *PolicyGuard, httpClient *http.Client, execLauncher ExecLauncher) *Handler {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Handler{
		guard:        guard,
		httpClient:   httpClient,
		execLauncher: execLauncher,
	}
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
	info, err := os.Stat(resolved.RealPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ListDirResponse{}, fmt.Errorf("%w: %s", ErrNotFound, req.Path)
		}
		return ListDirResponse{}, err
	}
	if !info.IsDir() {
		return ListDirResponse{}, fmt.Errorf("%w: path is not a directory", ErrInvalidRequest)
	}
	dirEntries, err := os.ReadDir(resolved.RealPath)
	if err != nil {
		return ListDirResponse{}, err
	}
	out := ListDirResponse{Entries: make([]DirEntry, 0, min(len(dirEntries), limit))}
	for _, entry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return ListDirResponse{}, err
		}
		childRel := joinRel(resolved.RelPath, entry.Name())
		var stat fs.FileInfo
		if entry.Type()&os.ModeSymlink != 0 {
			childResolved, err := h.guard.Resolve(req.GrantID, childRel, OpRead)
			if err != nil {
				continue
			}
			stat, err = os.Stat(childResolved.RealPath)
			if err != nil {
				continue
			}
		} else {
			if err := checkRelativeSegments(entry.Name()); err != nil {
				continue
			}
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
	grantRoot, err := h.guard.Resolve(req.GrantID, ".", OpRead)
	if err != nil {
		return SearchResponse{}, err
	}
	startInfo, err := os.Stat(resolved.RealPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SearchResponse{}, fmt.Errorf("%w: %s", ErrNotFound, req.Path)
		}
		return SearchResponse{}, err
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

	searchFile := func(absPath, relPath string, size int64) error {
		if strings.Contains(strings.ToLower(relPath), lowerQuery) {
			if err := addMatch(SearchMatch{RelPath: relPath}); err != nil {
				return err
			}
		}
		if size <= 0 || size > maxSearchFileBytes {
			return nil
		}
		data, err := os.ReadFile(absPath)
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
			if err := searchFile(resolved.RealPath, resolved.RelPath, startInfo.Size()); err != nil && !errors.Is(err, errSearchLimit) {
				return SearchResponse{}, err
			}
		}
		return out, nil
	}

	walkErr := filepath.WalkDir(resolved.RealPath, func(path string, d fs.DirEntry, walkErr error) error {
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
			if path != resolved.RealPath && shouldSkipSearchDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, ok := relWithin(grantRoot.RealPath, path)
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		return searchFile(path, rel, info.Size())
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
	file, err := os.Open(resolved.RealPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StagedFile{}, fmt.Errorf("%w: %s", ErrNotFound, resolved.RelPath)
		}
		return StagedFile{}, err
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
	applyTransferHeaders(httpReq, target.UploadHeaders)
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

func (h *Handler) CommitFiles(ctx context.Context, caps Capabilities, req DeviceCommitRequest) (DeviceCommitResponse, error) {
	if !caps.FileStaging {
		return DeviceCommitResponse{}, fmt.Errorf("%w: file staging disabled", ErrUnsupportedCapability)
	}
	resolvedPaths, err := h.guard.ValidateCommitRequest(req)
	if err != nil {
		return DeviceCommitResponse{}, err
	}
	for i, entry := range req.Files {
		if err := CheckCommitPrecondition(resolvedPaths[i].RealPath, entry.ExpectedMtimeMs, entry.ExpectedSHA256); err != nil {
			return DeviceCommitResponse{}, err
		}
	}
	committed := make([]CommittedFile, 0, len(req.Files))
	for i, entry := range req.Files {
		if err := ctx.Err(); err != nil {
			return DeviceCommitResponse{}, err
		}
		item, err := h.commitOne(ctx, caps.MaxStageBytes, resolvedPaths[i], entry)
		if err != nil {
			return DeviceCommitResponse{}, err
		}
		committed = append(committed, item)
	}
	return DeviceCommitResponse{Files: committed}, nil
}

func (h *Handler) commitOne(ctx context.Context, maxBytes int64, resolved ResolvedPath, entry CommitFileEntry) (CommittedFile, error) {
	if entry.Delete {
		info, err := os.Lstat(resolved.RealPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return CommittedFile{}, fmt.Errorf("%w: %s", ErrNotFound, resolved.RelPath)
			}
			return CommittedFile{}, err
		}
		if info.IsDir() {
			return CommittedFile{}, fmt.Errorf("%w: cannot delete directory %s", ErrInvalidRequest, resolved.RelPath)
		}
		if err := os.Remove(resolved.RealPath); err != nil {
			return CommittedFile{}, err
		}
		return CommittedFile{RelPath: resolved.RelPath, Deleted: true}, nil
	}

	perm := fs.FileMode(0o644)
	if existing, err := os.Stat(resolved.RealPath); err == nil {
		if existing.IsDir() {
			return CommittedFile{}, fmt.Errorf("%w: cannot overwrite directory %s", ErrInvalidRequest, resolved.RelPath)
		}
		perm = existing.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return CommittedFile{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(entry.DownloadURL), nil)
	if err != nil {
		return CommittedFile{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	applyTransferHeaders(httpReq, entry.DownloadHeaders)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return CommittedFile{}, fmt.Errorf("download %s: %w", resolved.RelPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return CommittedFile{}, fmt.Errorf("download %s: unexpected HTTP status %d", resolved.RelPath, resp.StatusCode)
	}

	parentDir := filepath.Dir(resolved.RealPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return CommittedFile{}, err
	}
	tmp, err := os.CreateTemp(parentDir, ".jaz-commit-*")
	if err != nil {
		return CommittedFile{}, err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
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
		return CommittedFile{}, err
	}
	if maxBytes > 0 && written > maxBytes {
		_ = tmp.Close()
		return CommittedFile{}, fmt.Errorf("%w: %s download exceeds max_stage_bytes %d", ErrInvalidRequest, resolved.RelPath, maxBytes)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return CommittedFile{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return CommittedFile{}, err
	}
	if err := tmp.Close(); err != nil {
		return CommittedFile{}, err
	}

	if err := CheckCommitPrecondition(resolved.RealPath, entry.ExpectedMtimeMs, entry.ExpectedSHA256); err != nil {
		return CommittedFile{}, err
	}
	if err := os.Rename(tmpPath, resolved.RealPath); err != nil {
		return CommittedFile{}, err
	}
	cleanup = false

	info, err := os.Stat(resolved.RealPath)
	if err != nil {
		return CommittedFile{}, err
	}
	return CommittedFile{
		RelPath:   resolved.RelPath,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
		SizeBytes: written,
		MtimeMs:   info.ModTime().UnixMilli(),
	}, nil
}

func (h *Handler) ExecStart(ctx context.Context, caps Capabilities, req ExecStartRequest) (ExecStartResponse, error) {
	if !caps.LocalExec || h.execLauncher == nil {
		return ExecStartResponse{}, fmt.Errorf("%w: local exec disabled", ErrUnsupportedCapability)
	}
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" {
		return ExecStartResponse{}, fmt.Errorf("%w: grant_id and run_id are required", ErrInvalidRequest)
	}
	resolved, err := h.guard.Resolve(req.GrantID, ".", OpRead)
	if err != nil {
		return ExecStartResponse{}, err
	}
	return h.execLauncher.Start(ctx, req, resolved.RealPath)
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

func applyTransferHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		key := strings.TrimSpace(k)
		if key == "" || strings.EqualFold(key, "Content-Length") {
			continue
		}
		if strings.EqualFold(key, "Host") {
			req.Host = v
			continue
		}
		req.Header.Set(key, v)
	}
}

type byteCounter struct {
	n int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
