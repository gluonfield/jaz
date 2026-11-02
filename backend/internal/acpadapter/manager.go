package acpadapter

import (
	"context"
	"net/http"
	"os"
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
}

type Manager struct {
	root      string
	manifest  manifest
	client    *http.Client
	installMu sync.Mutex
	mu        sync.Mutex
	status    map[string]Status
}

func New(root string) *Manager {
	return &Manager{
		root:     root,
		manifest: parsePinnedManifest(),
		client:   &http.Client{Timeout: 10 * time.Minute},
		status:   map[string]Status{},
	}
}

func (m *Manager) ResolveAdapter(ctx context.Context, name string) (acp.AdapterLaunch, error) {
	name = strings.TrimSpace(name)
	spec, err := m.resolveSpec(name)
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
	if spec, err := m.resolveSpec(name); err == nil && m.installed(spec) {
		return readyStatus(spec)
	}
	return Status{
		Adapter:  name,
		Platform: platform,
		State:    StateMissing,
	}
}

func (m *Manager) Prepare(ctx context.Context, name string) error {
	_, err := m.ResolveAdapter(ctx, name)
	return err
}

func (m *Manager) installed(spec adapterSpec) bool {
	for _, path := range spec.executables() {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || (!strings.HasPrefix(spec.Platform, "win32-") && info.Mode().Perm()&0o111 == 0) {
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
		Adapter:  adapter,
		Platform: platform,
		State:    state,
		Message:  err.Error(),
	})
}

func downloadingStatus(spec adapterSpec) Status {
	return Status{
		Adapter:  spec.Adapter,
		Version:  spec.Version,
		Platform: spec.Platform,
		Path:     spec.Command,
		State:    StateDownloading,
	}
}

func readyStatus(spec adapterSpec) Status {
	return Status{
		Adapter:  spec.Adapter,
		Version:  spec.Version,
		Platform: spec.Platform,
		Path:     spec.Command,
		State:    StateReady,
	}
}

func failedStatus(spec adapterSpec, err error) Status {
	return Status{
		Adapter:  spec.Adapter,
		Version:  spec.Version,
		Platform: spec.Platform,
		Path:     spec.Command,
		State:    StateFailed,
		Message:  err.Error(),
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
