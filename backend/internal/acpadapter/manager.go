package acpadapter

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
)

const (
	StateMissing     = "missing"
	StateDownloading = "downloading"
	StateReady       = "ready"
	StateFailed      = "failed"
	StateUnsupported = "unsupported"
)

type Status struct {
	Adapter         string
	Version         string
	Platform        string
	Path            string
	State           string
	Message         string
	BytesDownloaded int64
	BytesTotal      int64
	ProgressPercent int
	StartedAt       time.Time
	FinishedAt      time.Time
}

type Manager struct {
	root              string
	manifestURL       string
	manifestCacheName string
	localManifestPath string
	client            *http.Client
	installMu         sync.Mutex
	manifestMu        sync.Mutex
	mu                sync.Mutex
	manifest          manifest
	hasManifest       bool
	status            map[string]Status
}

func New(root, releaseVersion string) *Manager {
	localManifestPath := ""
	if usesLatestManifest(releaseVersion) {
		localManifestPath = findLocalManifestPath()
	}
	return &Manager{
		root:              root,
		manifestURL:       manifestURLForVersion(releaseVersion),
		manifestCacheName: manifestCacheNameForVersion(releaseVersion),
		localManifestPath: localManifestPath,
		client:            &http.Client{Timeout: 10 * time.Minute},
		status:            map[string]Status{},
	}
}

func NewForTest(root, manifestURL string, client *http.Client) *Manager {
	m := New(root, "dev")
	m.manifestURL = strings.TrimSpace(manifestURL)
	m.manifestCacheName = "test"
	m.localManifestPath = ""
	if client != nil {
		m.client = client
	}
	return m
}

func (m *Manager) ResolveAdapter(ctx context.Context, name string) (acp.AdapterLaunch, error) {
	name = strings.TrimSpace(name)
	spec, err := m.resolveSpec(ctx, name)
	if err != nil {
		m.setResolveErrorStatus(name, err)
		return acp.AdapterLaunch{}, err
	}
	if m.installed(spec) {
		m.setStatus(name, readyStatus(spec))
		return spec.launch(), nil
	}
	m.setStatus(name, downloadingStatus(spec))
	m.installMu.Lock()
	defer m.installMu.Unlock()
	if m.installed(spec) {
		m.setStatus(name, readyStatus(spec))
		return spec.launch(), nil
	}
	if err := m.installArchive(ctx, spec); err != nil {
		m.setStatus(name, failedStatus(spec, err))
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

func (m *Manager) setDownloadProgress(spec adapterSpec, downloaded, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.status[spec.Adapter]
	if !ok || status.State != StateDownloading || status.Version != spec.Version || status.Platform != spec.Platform {
		return
	}
	status.BytesDownloaded = downloaded
	status.BytesTotal = total
	status.ProgressPercent = progressPercent(downloaded, total)
	m.status[spec.Adapter] = status
}

func (m *Manager) setResolveErrorStatus(adapter string, err error) {
	platform, platformErr := platformKey(runtime.GOOS, runtime.GOARCH)
	state := StateFailed
	if platformErr != nil {
		state = StateUnsupported
	}
	m.setStatus(adapter, Status{
		Adapter:    adapter,
		Platform:   platform,
		State:      state,
		Message:    err.Error(),
		FinishedAt: time.Now().UTC(),
	})
}

func downloadingStatus(spec adapterSpec) Status {
	return Status{
		Adapter:   spec.Adapter,
		Version:   spec.Version,
		Platform:  spec.Platform,
		Path:      spec.Command,
		State:     StateDownloading,
		Message:   "Downloading " + displayName(spec.Adapter) + " adapter",
		StartedAt: time.Now().UTC(),
	}
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

func failedStatus(spec adapterSpec, err error) Status {
	return Status{
		Adapter:    spec.Adapter,
		Version:    spec.Version,
		Platform:   spec.Platform,
		Path:       spec.Command,
		State:      StateFailed,
		Message:    err.Error(),
		FinishedAt: time.Now().UTC(),
	}
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

func progressPercent(downloaded, total int64) int {
	if downloaded <= 0 || total <= 0 {
		return 0
	}
	percent := int(downloaded * 100 / total)
	if percent > 100 {
		return 100
	}
	return percent
}
