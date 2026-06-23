package acpadapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
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

const npmRegistry = "https://registry.npmjs.org"

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
	root      string
	registry  string
	client    *http.Client
	installMu sync.Mutex
	mu        sync.Mutex
	status    map[string]Status
}

func New(root string) *Manager {
	return &Manager{
		root:     root,
		registry: npmRegistry,
		client:   &http.Client{Timeout: 2 * time.Minute},
		status:   map[string]Status{},
	}
}

func NewForTest(root, registry string, client *http.Client) *Manager {
	m := New(root)
	m.registry = strings.TrimRight(registry, "/")
	if client != nil {
		m.client = client
	}
	return m
}

func (m *Manager) ResolveAdapter(ctx context.Context, name string) (acp.AdapterLaunch, error) {
	switch strings.TrimSpace(name) {
	case "codex":
		return m.resolveCodex(ctx)
	default:
		return acp.AdapterLaunch{}, fmt.Errorf("unknown managed acp adapter %q", name)
	}
}

func (m *Manager) Status(name string) Status {
	switch strings.TrimSpace(name) {
	case "codex":
		return m.codexStatus()
	default:
		return Status{
			Adapter: strings.TrimSpace(name),
			State:   StateUnsupported,
			Message: fmt.Sprintf("unknown managed acp adapter %q", strings.TrimSpace(name)),
		}
	}
}

func (m *Manager) Prepare(ctx context.Context, name string) error {
	_, err := m.ResolveAdapter(ctx, name)
	return err
}

func (m *Manager) resolveCodex(ctx context.Context) (acp.AdapterLaunch, error) {
	spec, err := codexSpec(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return acp.AdapterLaunch{}, err
	}
	path := m.adapterPath("codex", acp.CodexACPVersion, spec.Platform, spec.BinaryName)
	if executableExists(path) {
		m.setStatus("codex", readyStatus(spec, path))
		return acp.AdapterLaunch{Command: path}, nil
	}
	m.setStatus("codex", Status{
		Adapter:   "codex",
		Version:   acp.CodexACPVersion,
		Platform:  spec.Platform,
		Path:      path,
		State:     StateDownloading,
		Message:   "Downloading Codex adapter",
		StartedAt: time.Now().UTC(),
	})
	m.installMu.Lock()
	defer m.installMu.Unlock()
	if executableExists(path) {
		m.setStatus("codex", readyStatus(spec, path))
		return acp.AdapterLaunch{Command: path}, nil
	}
	if err := m.installNPMBinary(ctx, spec.Package, acp.CodexACPVersion, spec.TarPath, path); err != nil {
		m.setStatus("codex", Status{
			Adapter:    "codex",
			Version:    acp.CodexACPVersion,
			Platform:   spec.Platform,
			Path:       path,
			State:      StateFailed,
			Message:    err.Error(),
			FinishedAt: time.Now().UTC(),
		})
		return acp.AdapterLaunch{}, err
	}
	m.setStatus("codex", readyStatus(spec, path))
	return acp.AdapterLaunch{Command: path}, nil
}

func (m *Manager) codexStatus() Status {
	spec, err := codexSpec(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Status{
			Adapter: "codex",
			Version: acp.CodexACPVersion,
			State:   StateUnsupported,
			Message: err.Error(),
		}
	}
	path := m.adapterPath("codex", acp.CodexACPVersion, spec.Platform, spec.BinaryName)
	if executableExists(path) {
		return readyStatus(spec, path)
	}
	if status, ok := m.storedStatus("codex"); ok {
		return status
	}
	return Status{
		Adapter:  "codex",
		Version:  acp.CodexACPVersion,
		Platform: spec.Platform,
		Path:     path,
		State:    StateMissing,
		Message:  "Codex adapter is not downloaded yet",
	}
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

func readyStatus(spec binarySpec, path string) Status {
	return Status{
		Adapter:    "codex",
		Version:    acp.CodexACPVersion,
		Platform:   spec.Platform,
		Path:       path,
		State:      StateReady,
		Message:    "Codex adapter is ready",
		FinishedAt: time.Now().UTC(),
	}
}

func (m *Manager) adapterPath(agent, version, platform, binary string) string {
	return filepath.Join(m.root, "acp", "managed", "adapters", agent, version, platform, binary)
}

func (m *Manager) installNPMBinary(ctx context.Context, pkg, version, tarPath, dst string) error {
	meta, err := m.npmVersion(ctx, pkg, version)
	if err != nil {
		return err
	}
	body, err := m.download(ctx, meta.Dist.Tarball)
	if err != nil {
		return err
	}
	if err := verifyIntegrity(body, meta.Dist.Integrity); err != nil {
		return fmt.Errorf("verify %s@%s: %w", pkg, version, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, filepath.Base(dst))
	if err := extractOne(body, tarPath, out); err != nil {
		return err
	}
	return os.Rename(out, dst)
}

func (m *Manager) npmVersion(ctx context.Context, pkg, version string) (npmPackageVersion, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.registry+"/"+escapedPackage(pkg)+"/"+version, nil)
	if err != nil {
		return npmPackageVersion{}, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return npmPackageVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return npmPackageVersion{}, fmt.Errorf("fetch npm metadata for %s@%s: %s", pkg, version, res.Status)
	}
	var meta npmPackageVersion
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		return npmPackageVersion{}, err
	}
	if strings.TrimSpace(meta.Dist.Tarball) == "" || strings.TrimSpace(meta.Dist.Integrity) == "" {
		return npmPackageVersion{}, fmt.Errorf("npm metadata for %s@%s missing tarball integrity", pkg, version)
	}
	return meta, nil
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

type npmPackageVersion struct {
	Dist struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

type binarySpec struct {
	Package    string
	Platform   string
	TarPath    string
	BinaryName string
}

func codexSpec(goos, goarch string) (binarySpec, error) {
	arch := ""
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return binarySpec{}, fmt.Errorf("unsupported codex adapter architecture %s", goarch)
	}
	osName := ""
	binary := "codex-acp"
	switch goos {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "win32"
		binary += ".exe"
	default:
		return binarySpec{}, fmt.Errorf("unsupported codex adapter OS %s", goos)
	}
	platform := osName + "-" + arch
	return binarySpec{
		Package:    "@jazchat/codex-acp-" + platform,
		Platform:   platform,
		TarPath:    "package/bin/" + binary,
		BinaryName: binary,
	}, nil
}

func escapedPackage(pkg string) string {
	return strings.ReplaceAll(pkg, "/", "%2f")
}

func verifyIntegrity(body []byte, integrity string) error {
	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(integrity), "-")
	if !ok || algorithm != "sha512" || encoded == "" {
		return fmt.Errorf("unsupported integrity %q", integrity)
	}
	want, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	got := sha512.Sum512(body)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("sha512 mismatch")
	}
	return nil
}

func extractOne(body []byte, tarPath, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("adapter tarball missing %s", tarPath)
		}
		if err != nil {
			return err
		}
		if path.Clean(header.Name) != tarPath || header.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	}
}

func executableExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
