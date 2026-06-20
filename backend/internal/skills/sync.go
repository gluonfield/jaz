package skills

import (
	"archive/zip"
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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	firstPartySkillSource = "jaz-skills"
	syncStateFile         = "skills.sync.json"
	maxExpandedArchive    = 512 << 20
)

type RemoteSyncConfig struct {
	ManifestURL     string
	ManifestSHA256  string
	Client          *http.Client
	MaxManifestSize int64
	MaxArchiveSize  int64
}

type RemoteManifest struct {
	Version string        `json:"version"`
	Skills  []RemoteSkill `json:"skills"`
}

type RemoteSkill struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	ArchiveURL string `json:"archive_url,omitempty"`
	URL        string `json:"url,omitempty"`
	SHA256     string `json:"sha256"`
}

type managedSkill struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256"`
}

func SyncRemote(ctx context.Context, root string, cfg RemoteSyncConfig) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("runtime root is empty")
	}
	manifestURL := strings.TrimSpace(cfg.ManifestURL)
	if err := validateHTTPURL(manifestURL); err != nil {
		return fmt.Errorf("skills manifest url: %w", err)
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	manifestLimit := cfg.MaxManifestSize
	if manifestLimit <= 0 {
		manifestLimit = 4 << 20
	}
	body, err := fetchBytes(ctx, client, manifestURL, manifestLimit)
	if err != nil {
		return err
	}
	if expected := strings.ToLower(strings.TrimSpace(cfg.ManifestSHA256)); expected != "" {
		if _, err := hex.DecodeString(expected); err != nil || len(expected) != sha256.Size*2 {
			return fmt.Errorf("invalid manifest sha256")
		}
		sum := sha256.Sum256(body)
		actual := hex.EncodeToString(sum[:])
		if actual != expected {
			return fmt.Errorf("manifest sha256 mismatch: got %s", actual)
		}
	}
	var manifest RemoteManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return fmt.Errorf("decode skills manifest: %w", err)
	}
	var errs []error
	for _, skill := range manifest.Skills {
		if err := syncRemoteSkill(ctx, root, client, cfg, skill); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", firstNonEmpty(skill.Name, "skill"), err))
		}
	}
	return errors.Join(errs...)
}

func syncRemoteSkill(ctx context.Context, root string, client *http.Client, cfg RemoteSyncConfig, skill RemoteSkill) error {
	name := strings.TrimSpace(skill.Name)
	if !validSkillName(name) {
		return fmt.Errorf("invalid skill name %q", skill.Name)
	}
	archiveURL := strings.TrimSpace(firstNonEmpty(skill.ArchiveURL, skill.URL))
	if err := validateHTTPURL(archiveURL); err != nil {
		return fmt.Errorf("archive url: %w", err)
	}
	expected := strings.ToLower(strings.TrimSpace(skill.SHA256))
	if _, err := hex.DecodeString(expected); err != nil || len(expected) != sha256.Size*2 {
		return fmt.Errorf("invalid sha256")
	}
	archiveLimit := cfg.MaxArchiveSize
	if archiveLimit <= 0 {
		archiveLimit = 128 << 20
	}
	archive, err := fetchBytes(ctx, client, archiveURL, archiveLimit)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("sha256 mismatch: got %s", actual)
	}
	return installRemoteArchive(root, skill, archive, actual)
}

func installRemoteArchive(root string, skill RemoteSkill, archive []byte, sha string) error {
	userRoot := UserRoot(root)
	if err := os.MkdirAll(userRoot, 0o755); err != nil {
		return err
	}
	name := strings.TrimSpace(skill.Name)
	work, err := os.MkdirTemp(userRoot, "."+name+"-sync-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := extractSkillZip(archive, work); err != nil {
		return err
	}
	skillRoot, err := archiveSkillRoot(work)
	if err != nil {
		return err
	}
	extracted, ok := readSkill(filepath.Join(skillRoot, "SKILL.md"))
	if !ok || !strings.EqualFold(extracted.Name, name) {
		return fmt.Errorf("archive skill name does not match manifest name %q", name)
	}

	tmp, err := os.MkdirTemp(userRoot, "."+name+"-install-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := copyDir(skillRoot, tmp); err != nil {
		return err
	}

	dest := filepath.Join(userRoot, name)
	installMu.Lock()
	defer installMu.Unlock()
	state, err := loadSyncState(root)
	if err != nil {
		return err
	}
	replace, err := canReplaceSkill(dest, state, name)
	if err != nil || !replace {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	state[name] = managedSkill{
		Source:  firstPartySkillSource,
		Name:    name,
		Version: strings.TrimSpace(skill.Version),
		SHA256:  sha,
	}
	return saveSyncState(root, state)
}

func canReplaceSkill(dest string, state map[string]managedSkill, name string) (bool, error) {
	info, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("skill path is not a directory: %s", dest)
	}
	if state[name].Source == firstPartySkillSource {
		return true, nil
	}
	return false, nil
}

func loadSyncState(root string) (map[string]managedSkill, error) {
	data, err := os.ReadFile(filepath.Join(root, syncStateFile))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]managedSkill{}, nil
		}
		return nil, err
	}
	var state map[string]managedSkill
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("read skill sync state: %w", err)
	}
	if state == nil {
		state = map[string]managedSkill{}
	}
	return state, nil
}

func saveSyncState(root string, state map[string]managedSkill) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, syncStateFile), append(data, '\n'), 0o644)
}

func extractSkillZip(data []byte, dst string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read skill zip: %w", err)
	}
	var expanded uint64
	for _, file := range reader.File {
		rel, ok := cleanZipPath(file.Name)
		if !ok {
			return fmt.Errorf("unsafe zip path %q", file.Name)
		}
		target := filepath.Join(dst, rel)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe zip path %q", file.Name)
		}
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in skill archive: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("unsupported archive entry: %s", file.Name)
		}
		expanded += file.UncompressedSize64
		if expanded > maxExpandedArchive {
			return fmt.Errorf("expanded skill archive exceeds %d bytes", maxExpandedArchive)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		if err := writeExtractedFile(target, src, mode.Perm()); err != nil {
			_ = src.Close()
			return err
		}
		if err := src.Close(); err != nil {
			return err
		}
	}
	return nil
}

func writeExtractedFile(path string, src io.Reader, perm fs.FileMode) error {
	if perm == 0 {
		perm = 0o644
	}
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

func archiveSkillRoot(root string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return root, nil
	}
	var roots []string
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			roots = append(roots, filepath.Dir(p))
		}
		return nil
	}); err != nil {
		return "", err
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("skill archive must contain exactly one SKILL.md")
	}
	return roots[0], nil
}

func fetchBytes(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s", rawURL, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", rawURL, limit)
	}
	return data, nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func cleanZipPath(name string) (string, bool) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false
	}
	return filepath.FromSlash(clean), true
}

func validSkillName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
