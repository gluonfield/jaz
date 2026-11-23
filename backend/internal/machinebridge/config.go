package machinebridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const MaxFolderGrants = 32

type ShareConfig struct {
	Enabled     bool          `json:"enabled"`
	DisplayName string        `json:"display_name"`
	Grants      []FolderGrant `json:"grants"`
	AllowExec   bool          `json:"allow_exec"`
}

type BridgeStatus struct {
	DeviceID        string      `json:"device_id"`
	Connected       bool        `json:"connected"`
	CloudURL        string      `json:"cloud_url,omitempty"`
	LastConnectedAt *time.Time  `json:"last_connected_at,omitempty"`
	LastError       string      `json:"last_error,omitempty"`
	Config          ShareConfig `json:"config"`
}

type Controller interface {
	Status() BridgeStatus
	UpdateConfig(ctx context.Context, cfg ShareConfig) (BridgeStatus, error)
}

func DefaultDisplayName(fallback string) string {
	if host, err := os.Hostname(); err == nil {
		host = strings.TrimSpace(host)
		if host != "" && !containsControlChar(host) {
			return host
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" && !containsControlChar(fallback) {
		return fallback
	}
	return "desktop"
}

func DefaultShareConfig(workspace, fallbackName string) ShareConfig {
	grants := []FolderGrant{}
	workspace = strings.TrimSpace(workspace)
	if workspace != "" && filepath.IsAbs(workspace) && !containsControlChar(workspace) {
		cleaned := filepath.Clean(workspace)
		if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
			grants = append(grants, FolderGrant{
				ID:       "workspace",
				Mode:     AccessReadWrite,
				RootPath: cleaned,
			})
		}
	}
	return ShareConfig{
		Enabled:     true,
		DisplayName: DefaultDisplayName(fallbackName),
		Grants:      grants,
		AllowExec:   false,
	}
}

func NormalizeShareConfig(cfg ShareConfig, fallbackName string) (ShareConfig, error) {
	if containsControlChar(cfg.DisplayName) {
		return ShareConfig{}, fmt.Errorf("%w: display_name cannot contain control characters", ErrInvalidRequest)
	}
	displayName := strings.TrimSpace(cfg.DisplayName)
	if displayName == "" {
		displayName = DefaultDisplayName(fallbackName)
	}
	if len(cfg.Grants) > MaxFolderGrants {
		return ShareConfig{}, fmt.Errorf("%w: at most %d folder grants are allowed", ErrInvalidRequest, MaxFolderGrants)
	}

	usedIDs := make(map[string]struct{}, len(cfg.Grants))
	explicitIDs := make([]string, len(cfg.Grants))
	for i, grant := range cfg.Grants {
		if containsControlChar(grant.ID) {
			return ShareConfig{}, fmt.Errorf("%w: grant id cannot contain control characters", ErrInvalidRequest)
		}
		id := strings.TrimSpace(grant.ID)
		if id == "" {
			continue
		}
		if strings.ContainsAny(id, "=:") {
			return ShareConfig{}, fmt.Errorf("%w: grant id %q cannot contain '=' or ':'", ErrInvalidRequest, id)
		}
		if _, exists := usedIDs[id]; exists {
			return ShareConfig{}, fmt.Errorf("%w: duplicate grant id %q", ErrInvalidRequest, id)
		}
		usedIDs[id] = struct{}{}
		explicitIDs[i] = id
	}

	grants := make([]FolderGrant, 0, len(cfg.Grants))
	for i, grant := range cfg.Grants {
		if containsControlChar(grant.RootPath) {
			return ShareConfig{}, fmt.Errorf("%w: folder path cannot contain control characters", ErrInvalidRequest)
		}
		rootPath := strings.TrimSpace(grant.RootPath)
		if rootPath == "" || !filepath.IsAbs(rootPath) {
			return ShareConfig{}, fmt.Errorf("%w: grant root must be an absolute path", ErrInvalidRequest)
		}
		cleaned := filepath.Clean(rootPath)
		info, err := os.Stat(cleaned)
		if err != nil {
			return ShareConfig{}, fmt.Errorf("%w: folder %q: %v", ErrInvalidRequest, cleaned, err)
		}
		if !info.IsDir() {
			return ShareConfig{}, fmt.Errorf("%w: folder %q is not a directory", ErrInvalidRequest, cleaned)
		}
		mode := AccessMode(strings.TrimSpace(string(grant.Mode)))
		if !mode.Valid() {
			return ShareConfig{}, fmt.Errorf("%w: invalid access mode %q", ErrInvalidRequest, grant.Mode)
		}
		if containsControlChar(grant.Label) {
			return ShareConfig{}, fmt.Errorf("%w: grant label cannot contain control characters", ErrInvalidRequest)
		}
		id := explicitIDs[i]
		if id == "" {
			id = deriveGrantID(cleaned, usedIDs)
			usedIDs[id] = struct{}{}
		}
		grants = append(grants, FolderGrant{
			ID:       id,
			RootPath: cleaned,
			Mode:     mode,
			Label:    strings.TrimSpace(grant.Label),
		})
	}

	return ShareConfig{
		Enabled:     cfg.Enabled,
		DisplayName: displayName,
		Grants:      grants,
		AllowExec:   cfg.AllowExec,
	}, nil
}

func (c ShareConfig) Clone() ShareConfig {
	out := c
	if len(c.Grants) == 0 {
		out.Grants = []FolderGrant{}
	} else {
		out.Grants = slices.Clone(c.Grants)
	}
	return out
}

func (c ShareConfig) Hash() string {
	canonical := c.Clone()
	slices.SortFunc(canonical.Grants, func(a, b FolderGrant) int {
		if cmp := strings.Compare(a.ID, b.ID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.RootPath, b.RootPath)
	})
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func containsControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func deriveGrantID(rootPath string, used map[string]struct{}) string {
	base := slugGrantID(filepath.Base(rootPath))
	if _, exists := used[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func slugGrantID(leaf string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(leaf) {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "folder"
	}
	return slug
}
