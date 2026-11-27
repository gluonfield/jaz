package appbundle

import (
	"encoding/json"
	"errors"
)

const (
	Format       = "jaz-app/1"
	ManifestPath = "manifest.json"

	KindArtifact = "artifact"
	KindDocument = "document"

	MaxBundleBytes int64 = 25 << 20 // 25 MiB uncompressed total
	MaxFileBytes   int64 = 10 << 20 // 10 MiB per file
	MaxFileCount         = 200
	MaxTitleRunes        = 200
	MaxDescRunes         = 2000
	MaxAllowedTools      = 50
	MaxCapabilities      = 32

	CapStoragePersonal    = "storage:personal"
	CapStorageShared      = "storage:shared"
	CapAIComplete         = "ai:complete"
	CapToolsCall          = "tools:call"
	CapDesktopWindow      = "desktop:window"
	CapDesktopNotifs      = "desktop:notifications"
	CapDesktopShortcuts   = "desktop:shortcuts"
	CapDesktopTray        = "desktop:tray"
	CapDesktopAlwaysOnTop = "desktop:always_on_top"
	CapDesktopFS          = "desktop:fs"
	CapDesktopFullAccess  = "desktop:full_access"
)

var (
	ErrInvalidManifest = errors.New("invalid jaz-app/1 manifest")
	ErrInvalidPath     = errors.New("invalid bundle file path")
	ErrSymlinkBlocked  = errors.New("symlinks and special files are not allowed in bundles")
	ErrBundleTooLarge  = errors.New("bundle exceeds 25 MiB uncompressed limit")
	ErrFileTooLarge    = errors.New("bundle file exceeds 10 MiB limit")
	ErrTooManyFiles    = errors.New("bundle exceeds 200 file limit")
	ErrDuplicatePath   = errors.New("duplicate or case-colliding bundle path")
	ErrMissingEntry    = errors.New("manifest entry file is missing from bundle")
)

var validCapabilities = map[string]bool{
	CapStoragePersonal:    true,
	CapStorageShared:      true,
	CapAIComplete:         true,
	CapToolsCall:          true,
	CapDesktopWindow:      true,
	CapDesktopNotifs:      true,
	CapDesktopShortcuts:   true,
	CapDesktopTray:        true,
	CapDesktopAlwaysOnTop: true,
	CapDesktopFS:          true,
	CapDesktopFullAccess:  true,
}

// RequiresPublishApproval reports whether an app asks for more than sandboxed
// storage: AI credits, MCP tools or desktop privileges, which the user approves
// on a staged bundle.
func RequiresPublishApproval(capabilities []string) bool {
	for _, capability := range capabilities {
		if capability != CapStoragePersonal && capability != CapStorageShared {
			return true
		}
	}
	return false
}

var allowlistedCDNHosts = map[string]bool{
	"cdnjs.cloudflare.com": true,
	"esm.sh":               true,
	"cdn.jsdelivr.net":     true,
	"unpkg.com":            true,
	"fonts.googleapis.com": true,
	"fonts.gstatic.com":    true,
}

type IconSpec struct {
	Kind       string `json:"kind,omitempty"`
	Value      string `json:"value,omitempty"`
	Color      string `json:"color,omitempty"`
	Background string `json:"background,omitempty"`
}

type Manifest struct {
	Format       string    `json:"format"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Kind         string    `json:"kind"`
	Entry        string    `json:"entry"`
	Capabilities []string  `json:"capabilities,omitempty"`
	AllowedTools []string  `json:"allowed_tools,omitempty"`
	Icon         *IconSpec `json:"icon,omitempty"`
}

type FileEntry struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"-"`
}

type ExternalURL struct {
	URL         string `json:"url"`
	Host        string `json:"host"`
	Allowlisted bool   `json:"allowlisted"`
	SourceFile  string `json:"source_file,omitempty"`
}

type Bundle struct {
	Manifest     Manifest        `json:"manifest"`
	ManifestJSON json.RawMessage `json:"-"`
	BundleSHA256 string          `json:"bundle_sha256"`
	SizeBytes    int64           `json:"size_bytes"`
	FileCount    int             `json:"file_count"`
	Files        []FileEntry     `json:"files"`
	ExternalURLs []ExternalURL   `json:"external_urls"`
	Warnings     []string        `json:"warnings"`
}
