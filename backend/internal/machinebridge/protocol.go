package machinebridge

import (
	"errors"
	"slices"
	"strings"
)

type ProtocolVersion int

const CurrentProtocolVersion ProtocolVersion = 1

const (
	PlatformDarwin  = "darwin"
	PlatformWindows = "windows"
	PlatformLinux   = "linux"
)

const (
	MethodHello           = "machine.hello"
	MethodListDir         = "device_list_dir"
	MethodSearch          = "device_search"
	MethodStageFiles      = "device_stage_files"
	MethodCommitFiles     = "device_commit_files"
	MethodExecStart       = "device_exec_start"
	MethodExecStop        = "device_exec_stop"
	MethodMCPCall         = "device_mcp_call"
	MethodExecTunnelOpen  = "exec_tunnel_open"
	MethodExecTunnelFrame = "exec_tunnel_frame"
	MethodExecTunnelClose = "exec_tunnel_close"
)

const (
	ExecTunnelMessageText   = "text"
	ExecTunnelMessageBinary = "binary"
	MaxExecTunnelFrameBytes = 2 << 20
)

type ErrorCode string

const (
	CodeDeviceOffline         ErrorCode = "device_offline"
	CodeAccessDenied          ErrorCode = "access_denied"
	CodePathOutsideGrant      ErrorCode = "path_outside_grant"
	CodeSymlinkEscape         ErrorCode = "symlink_escape"
	CodeConflict              ErrorCode = "conflict"
	CodeUnsupportedCapability ErrorCode = "unsupported_capability"
	CodeInvalidRequest        ErrorCode = "invalid_request"
	CodeNotFound              ErrorCode = "not_found"
	CodeInternal              ErrorCode = "internal_error"
)

var (
	ErrDeviceOffline         = errors.New("device_offline")
	ErrAccessDenied          = errors.New("access_denied")
	ErrPathOutsideGrant      = errors.New("path_outside_grant")
	ErrSymlinkEscape         = errors.New("symlink_escape")
	ErrConflict              = errors.New("conflict")
	ErrUnsupportedCapability = errors.New("unsupported_capability")
	ErrInvalidRequest        = errors.New("invalid_request")
	ErrNotFound              = errors.New("not_found")
)

type AccessMode string

const (
	AccessReadOnly        AccessMode = "ro"
	AccessReadWrite       AccessMode = "rw"
	AccessReadWriteDelete AccessMode = "rwd"
)

type Operation string

const (
	OpRead   Operation = "read"
	OpWrite  Operation = "write"
	OpDelete Operation = "delete"
)

func (m AccessMode) Valid() bool {
	return m == AccessReadOnly || m == AccessReadWrite || m == AccessReadWriteDelete
}

func (m AccessMode) Allows(op Operation) bool {
	switch op {
	case OpRead:
		return m.Valid()
	case OpWrite:
		return m == AccessReadWrite || m == AccessReadWriteDelete
	case OpDelete:
		return m == AccessReadWriteDelete
	default:
		return false
	}
}

type FolderGrant struct {
	ID       string     `json:"id"`
	RootPath string     `json:"root_path"`
	Mode     AccessMode `json:"mode"`
	Label    string     `json:"label,omitempty"`
}

type Capabilities struct {
	FileStaging   bool     `json:"file_staging,omitempty"`
	LocalExec     bool     `json:"local_exec,omitempty"`
	LocalBrowser  bool     `json:"local_browser,omitempty"`
	ComputerUse   bool     `json:"computer_use,omitempty"`
	LocalMCPs     []string `json:"local_mcps,omitempty"`
	MaxStageBytes int64    `json:"max_stage_bytes,omitempty"`
}

type HelloRequest struct {
	ProtocolVersion ProtocolVersion `json:"protocol_version"`
	DeviceID        string          `json:"device_id"`
	DisplayName     string          `json:"display_name"`
	Platform        string          `json:"platform"`
	AppVersion      string          `json:"app_version,omitempty"`
	Capabilities    Capabilities    `json:"capabilities"`
	FolderGrants    []FolderGrant   `json:"folder_grants,omitempty"`
}

type HelloResponse struct {
	ProtocolVersion ProtocolVersion `json:"protocol_version"`
	Accepted        bool            `json:"accepted"`
	Capabilities    Capabilities    `json:"capabilities,omitzero"`
	Error           string          `json:"error,omitempty"`
}

func ValidPlatform(platform string) bool {
	return platform == PlatformDarwin || platform == PlatformWindows || platform == PlatformLinux
}

func NegotiateCapabilities(client, server Capabilities) Capabilities {
	negotiated := Capabilities{
		FileStaging:  client.FileStaging && server.FileStaging,
		LocalExec:    client.LocalExec && server.LocalExec,
		LocalBrowser: client.LocalBrowser && server.LocalBrowser,
		ComputerUse:  client.ComputerUse && server.ComputerUse,
	}
	switch {
	case client.MaxStageBytes > 0 && server.MaxStageBytes > 0:
		negotiated.MaxStageBytes = min(client.MaxStageBytes, server.MaxStageBytes)
	case client.MaxStageBytes > 0:
		negotiated.MaxStageBytes = client.MaxStageBytes
	case server.MaxStageBytes > 0:
		negotiated.MaxStageBytes = server.MaxStageBytes
	}
	if len(client.LocalMCPs) > 0 && len(server.LocalMCPs) > 0 {
		allowed := make(map[string]struct{}, len(server.LocalMCPs))
		for _, id := range server.LocalMCPs {
			allowed[id] = struct{}{}
		}
		for _, id := range client.LocalMCPs {
			if _, ok := allowed[id]; ok && !slices.Contains(negotiated.LocalMCPs, id) {
				negotiated.LocalMCPs = append(negotiated.LocalMCPs, id)
			}
		}
	}
	return negotiated
}

func NegotiateHello(req HelloRequest, serverVersion ProtocolVersion, serverCaps Capabilities) HelloResponse {
	if req.ProtocolVersion <= 0 || serverVersion <= 0 {
		return HelloResponse{Error: "unsupported protocol version"}
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		return HelloResponse{Error: "device_id is required"}
	}
	if !ValidPlatform(req.Platform) {
		return HelloResponse{Error: "unsupported platform"}
	}
	version := min(req.ProtocolVersion, serverVersion)
	return HelloResponse{
		ProtocolVersion: version,
		Accepted:        true,
		Capabilities:    NegotiateCapabilities(req.Capabilities, serverCaps),
	}
}

type ListDirRequest struct {
	GrantID    string `json:"grant_id"`
	Path       string `json:"path,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

type DirEntry struct {
	RelPath   string `json:"rel_path"`
	Dir       bool   `json:"dir,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	MtimeMs   int64  `json:"mtime_ms,omitempty"`
}

type ListDirResponse struct {
	Entries   []DirEntry `json:"entries"`
	Truncated bool       `json:"truncated,omitempty"`
}

type SearchRequest struct {
	GrantID    string `json:"grant_id"`
	Path       string `json:"path,omitempty"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type SearchMatch struct {
	RelPath string `json:"rel_path"`
	Line    int    `json:"line,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type SearchResponse struct {
	Matches   []SearchMatch `json:"matches"`
	Truncated bool          `json:"truncated,omitempty"`
}

type StageFileTarget struct {
	RelPath       string            `json:"rel_path"`
	UploadURL     string            `json:"upload_url"`
	UploadHeaders map[string]string `json:"upload_headers,omitempty"`
	ContentType   string            `json:"content_type,omitempty"`
}

type DeviceStageRequest struct {
	GrantID string            `json:"grant_id"`
	RunID   string            `json:"run_id"`
	Files   []StageFileTarget `json:"files"`
}

type StagedFile struct {
	RelPath     string `json:"rel_path"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	MtimeMs     int64  `json:"mtime_ms"`
	ContentType string `json:"content_type,omitempty"`
}

type DeviceStageResponse struct {
	Files []StagedFile `json:"files"`
}

type CommitFileEntry struct {
	RelPath         string            `json:"rel_path"`
	DownloadURL     string            `json:"download_url,omitempty"`
	DownloadHeaders map[string]string `json:"download_headers,omitempty"`
	ExpectedMtimeMs *int64            `json:"expected_mtime_ms,omitempty"`
	ExpectedSHA256  string            `json:"expected_sha256,omitempty"`
	Delete          bool              `json:"delete,omitempty"`
}

type DeviceCommitRequest struct {
	GrantID string            `json:"grant_id"`
	RunID   string            `json:"run_id"`
	Files   []CommitFileEntry `json:"files"`
}

type CommittedFile struct {
	RelPath   string `json:"rel_path"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	MtimeMs   int64  `json:"mtime_ms,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type DeviceCommitResponse struct {
	Files []CommittedFile `json:"files"`
}

type ExecStartRequest struct {
	GrantID      string `json:"grant_id"`
	RunID        string `json:"run_id"`
	ReadOnlyDirs []string `json:"read_only_dirs,omitempty"`
}

type ExecStartResponse struct {
	SessionID string `json:"session_id"`
	Endpoint  string `json:"endpoint"`
	AuthToken string `json:"auth_token,omitempty"`
}

type ExecStopRequest struct {
	SessionID string `json:"session_id"`
}

type ExecStopResponse struct {
	Stopped bool `json:"stopped"`
}

type ExecTunnelOpenRequest struct {
	TunnelID  string `json:"tunnel_id"`
	SessionID string `json:"session_id"`
}

type ExecTunnelOpenResponse struct {
	OK bool `json:"ok"`
}

type ExecTunnelFrameParams struct {
	TunnelID    string `json:"tunnel_id"`
	MessageType string `json:"message_type"`
	Data        string `json:"data"`
}

type ExecTunnelCloseParams struct {
	TunnelID string `json:"tunnel_id"`
	Reason   string `json:"reason,omitempty"`
}
