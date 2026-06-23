package acpadapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
)

const defaultManifestURL = "https://github.com/gluonfield/jaz/releases/latest/download/acp-adapters.json"

const (
	StateMissing     = "missing"
	StateDownloading = "downloading"
	StateReady       = "ready"
	StateFailed      = "failed"
	StateUnsupported = "unsupported"
)

type Status struct {
	Adapter    string
	Version    string
	Platform   string
	Path       string
	State      string
	Message    string
	StartedAt  time.Time
	FinishedAt time.Time
}

type Manager struct {
	root        string
	manifestURL string
	client      *http.Client
	installMu   sync.Mutex
	manifestMu  sync.Mutex
	mu          sync.Mutex
	manifest    manifest
	hasManifest bool
	status      map[string]Status
}

func New(root string) *Manager {
	return &Manager{
		root:        root,
		manifestURL: defaultManifestURL,
		client:      &http.Client{Timeout: 10 * time.Minute},
		status:      map[string]Status{},
	}
}

func NewForTest(root, manifestURL string, client *http.Client) *Manager {
	m := New(root)
	m.manifestURL = strings.TrimSpace(manifestURL)
	if client != nil {
		m.client = client
	}
	return m
}

func (m *Manager) ResolveAdapter(ctx context.Context, name string) (acp.AdapterLaunch, error) {
	name = strings.TrimSpace(name)
	spec, err := m.resolveSpec(ctx, name)
	if err != nil {
		platform, platformErr := platformKey(runtime.GOOS, runtime.GOARCH)
		state := StateFailed
		if platformErr != nil {
			state = StateUnsupported
		}
		m.setStatus(name, Status{
			Adapter:    name,
			Platform:   platform,
			State:      state,
			Message:    err.Error(),
			FinishedAt: time.Now().UTC(),
		})
		return acp.AdapterLaunch{}, err
	}
	if m.installed(spec) {
		m.setStatus(name, readyStatus(spec))
		return spec.launch(), nil
	}
	m.setStatus(name, Status{
		Adapter:   name,
		Version:   spec.Version,
		Platform:  spec.Platform,
		Path:      spec.Command,
		State:     StateDownloading,
		Message:   "Downloading " + displayName(name) + " adapter",
		StartedAt: time.Now().UTC(),
	})
	m.installMu.Lock()
	defer m.installMu.Unlock()
	if m.installed(spec) {
		m.setStatus(name, readyStatus(spec))
		return spec.launch(), nil
	}
	if err := m.installArchive(ctx, spec); err != nil {
		m.setStatus(name, Status{
			Adapter:    name,
			Version:    spec.Version,
			Platform:   spec.Platform,
			Path:       spec.Command,
			State:      StateFailed,
			Message:    err.Error(),
			FinishedAt: time.Now().UTC(),
		})
		return acp.AdapterLaunch{}, err
	}
	m.setStatus(name, readyStatus(spec))
	return spec.launch(), nil
}

func (m *Manager) Status(name string) Status {
	name = strings.TrimSpace(name)
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Status{Adapter: name, State: StateUnsupported, Message: err.Error()}
	}
	if status, ok := m.storedStatus(name); ok {
		return status
	}
	return Status{
		Adapter:  name,
		Platform: platform,
		State:    StateMissing,
		Message:  displayName(name) + " adapter is not downloaded yet",
	}
}

func (m *Manager) Prepare(ctx context.Context, name string) error {
	_, err := m.ResolveAdapter(ctx, name)
	return err
}

func (m *Manager) resolveSpec(ctx context.Context, name string) (adapterSpec, error) {
	manifest, err := m.fetchManifest(ctx)
	if err != nil {
		return adapterSpec{}, err
	}
	adapter, ok := manifest.Adapters[name]
	if !ok {
		return adapterSpec{}, fmt.Errorf("managed acp adapter %q is not in the manifest", name)
	}
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return adapterSpec{}, err
	}
	asset, ok := adapter.Assets[platform]
	if !ok {
		return adapterSpec{}, fmt.Errorf("managed acp adapter %q does not support %s", name, platform)
	}
	if err := validateManifestAsset(name, adapter.Version, asset); err != nil {
		return adapterSpec{}, err
	}
	root := filepath.Join(m.root, "acp", "managed", "adapters", name, adapter.Version, platform)
	spec := adapterSpec{
		Adapter:  name,
		Version:  adapter.Version,
		Platform: platform,
		URL:      asset.URL,
		SHA256:   asset.SHA256,
		Root:     root,
		Command:  filepath.Join(root, filepath.FromSlash(asset.Binary)),
		Env:      map[string]string{},
	}
	for key, value := range asset.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		spec.Env[key] = resolveArchivePath(root, value)
	}
	return spec, nil
}

func (m *Manager) fetchManifest(ctx context.Context) (manifest, error) {
	if m.manifestURL == "" {
		return manifest{}, fmt.Errorf("managed acp adapter manifest URL is not configured")
	}
	out, err := m.fetchRemoteManifest(ctx)
	if err == nil {
		m.cacheManifest(out)
		_ = m.writeManifestCache(out)
		return out, nil
	}
	if cached, ok := m.cachedManifest(); ok {
		return cached, nil
	}
	if cached, ok := m.readManifestCache(); ok {
		m.cacheManifest(cached)
		return cached, nil
	}
	return manifest{}, err
}

func (m *Manager) fetchRemoteManifest(ctx context.Context) (manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.manifestURL, nil)
	if err != nil {
		return manifest{}, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return manifest{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return manifest{}, fmt.Errorf("fetch managed acp adapter manifest: %s", res.Status)
	}
	var out manifest
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return manifest{}, err
	}
	return out, nil
}

func (m *Manager) cacheManifest(out manifest) {
	m.manifestMu.Lock()
	defer m.manifestMu.Unlock()
	m.manifest = out
	m.hasManifest = true
}

func (m *Manager) cachedManifest() (manifest, bool) {
	m.manifestMu.Lock()
	defer m.manifestMu.Unlock()
	return m.manifest, m.hasManifest
}

func (m *Manager) writeManifestCache(out manifest) error {
	path := m.manifestCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func (m *Manager) readManifestCache() (manifest, bool) {
	body, err := os.ReadFile(m.manifestCachePath())
	if err != nil {
		return manifest{}, false
	}
	var out manifest
	if err := json.Unmarshal(body, &out); err != nil {
		return manifest{}, false
	}
	return out, true
}

func (m *Manager) manifestCachePath() string {
	return filepath.Join(m.root, "acp", "managed", "adapters", "manifest.json")
}

func (m *Manager) installArchive(ctx context.Context, spec adapterSpec) error {
	body, err := m.download(ctx, spec.URL)
	if err != nil {
		return err
	}
	if err := verifySHA256(body, spec.SHA256); err != nil {
		return fmt.Errorf("verify %s adapter archive: %w", spec.Adapter, err)
	}
	parent := filepath.Dir(spec.Root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := extractArchive(body, tmp); err != nil {
		return err
	}
	if err := chmodLaunchFiles(tmp, spec); err != nil {
		return err
	}
	_ = os.RemoveAll(spec.Root)
	return os.Rename(tmp, spec.Root)
}

func (m *Manager) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download adapter: %s", res.Status)
	}
	return io.ReadAll(res.Body)
}

func (m *Manager) storedStatus(adapter string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.status[adapter]
	return status, ok
}

func (m *Manager) setStatus(adapter string, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status[adapter] = status
}

type manifest struct {
	Adapters map[string]manifestAdapter `json:"adapters"`
}

type manifestAdapter struct {
	Version string                   `json:"version"`
	Assets  map[string]manifestAsset `json:"assets"`
}

type manifestAsset struct {
	URL    string            `json:"url"`
	SHA256 string            `json:"sha256"`
	Binary string            `json:"binary"`
	Env    map[string]string `json:"env,omitempty"`
}

type adapterSpec struct {
	Adapter  string
	Version  string
	Platform string
	URL      string
	SHA256   string
	Root     string
	Command  string
	Env      map[string]string
}

func (s adapterSpec) launch() acp.AdapterLaunch {
	return acp.AdapterLaunch{Command: s.Command, Env: s.Env}
}

func (m *Manager) installed(spec adapterSpec) bool {
	if !fileExists(spec.Command) {
		return false
	}
	for _, value := range spec.Env {
		if !fileExists(value) {
			return false
		}
	}
	return true
}

func readyStatus(spec adapterSpec) Status {
	return Status{
		Adapter:    spec.Adapter,
		Version:    spec.Version,
		Platform:   spec.Platform,
		Path:       spec.Command,
		State:      StateReady,
		Message:    displayName(spec.Adapter) + " adapter is ready",
		FinishedAt: time.Now().UTC(),
	}
}

func platformKey(goos, goarch string) (string, error) {
	arch := ""
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported managed adapter architecture %s", goarch)
	}
	switch goos {
	case "darwin":
		return "darwin-" + arch, nil
	case "linux":
		return "linux-" + arch, nil
	case "windows":
		return "win32-" + arch, nil
	default:
		return "", fmt.Errorf("unsupported managed adapter OS %s", goos)
	}
}

func validateManifestAsset(adapter, version string, asset manifestAsset) error {
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("managed acp adapter %q manifest entry is missing version", adapter)
	}
	if strings.TrimSpace(asset.URL) == "" || strings.TrimSpace(asset.SHA256) == "" || strings.TrimSpace(asset.Binary) == "" {
		return fmt.Errorf("managed acp adapter %q manifest entry is incomplete", adapter)
	}
	if !cleanRelative(asset.Binary) {
		return fmt.Errorf("managed acp adapter %q manifest binary path is invalid", adapter)
	}
	for key, value := range asset.Env {
		if strings.TrimSpace(key) == "" || !cleanRelative(value) {
			return fmt.Errorf("managed acp adapter %q manifest env path is invalid", adapter)
		}
	}
	return nil
}

func cleanRelative(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && path.Clean(value) == value && !path.IsAbs(value) && !strings.HasPrefix(value, "../")
}

func resolveArchivePath(root, value string) string {
	return filepath.Join(root, filepath.FromSlash(strings.TrimSpace(value)))
}

func verifySHA256(body []byte, wantHex string) error {
	want, err := hex.DecodeString(strings.TrimSpace(wantHex))
	if err != nil {
		return err
	}
	got := sha256.Sum256(body)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}

func extractArchive(body []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if !cleanRelative(name) {
			return fmt.Errorf("adapter archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func chmodLaunchFiles(tmpRoot string, spec adapterSpec) error {
	paths := []string{spec.Command}
	for _, value := range spec.Env {
		paths = append(paths, value)
	}
	for _, value := range paths {
		rel, err := filepath.Rel(spec.Root, value)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("adapter launch path escapes install root")
		}
		if err := os.Chmod(filepath.Join(tmpRoot, rel), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func displayName(adapter string) string {
	switch adapter {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	default:
		return adapter
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
