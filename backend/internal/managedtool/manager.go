package managedtool

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/execpath"
)

const (
	AntigravityCLI = "antigravity-cli"
	MuseCLI        = "muse"

	StateMissing     = "missing"
	StateDownloading = "downloading"
	StateReady       = "ready"
	StateFailed      = "failed"
	StateUnsupported = "unsupported"
)

const antigravityBaseURL = "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app"

type Status struct {
	Tool     string
	Version  string
	Platform string
	Path     string
	State    string
	Message  string
}

type Manager struct {
	root             string
	baseURL          string
	client           *http.Client
	installation     chan struct{}
	mu               sync.Mutex
	status           map[string]Status
	museInstallerURL string
}

func New(root string) *Manager {
	return &Manager{
		root:             root,
		baseURL:          antigravityBaseURL,
		client:           &http.Client{Timeout: 10 * time.Minute},
		installation:     make(chan struct{}, 1),
		status:           map[string]Status{},
		museInstallerURL: "https://dev.meta.ai/install.sh",
	}
}

func NewForTest(root, baseURL string, client *http.Client) *Manager {
	m := New(root)
	m.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if client != nil {
		m.client = client
	}
	return m
}

func (m *Manager) Prepare(ctx context.Context, name string, env map[string]string) error {
	name = strings.TrimSpace(name)
	if m.Status(name, env).State == StateReady {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.installation <- struct{}{}:
	}
	defer func() {
		<-m.installation
	}()
	if m.Status(name, env).State == StateReady {
		return nil
	}
	if name == MuseCLI {
		return m.prepareMuse(ctx)
	}
	spec, err := m.resolveSpec(ctx, name)
	if err != nil {
		m.setResolveErrorStatus(name, err)
		return err
	}
	m.setStatus(name, downloadingStatus(spec))
	if err := m.install(ctx, spec); err != nil {
		m.setStatus(name, failedStatus(spec, err))
		return err
	}
	m.setStatus(name, readyStatus(spec))
	return nil
}

func (m *Manager) Status(name string, env map[string]string) Status {
	name = strings.TrimSpace(name)
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return Status{Tool: name, State: StateUnsupported, Message: err.Error()}
	}
	if name == MuseCLI {
		return m.museStatus(platform, env)
	}
	if status, ok := m.storedStatus(name); ok {
		if status.State == StateDownloading {
			return status
		}
		if status.State == StateReady && fileExists(status.Path) {
			return status
		}
	}
	if status, ok := m.localReadyStatus(name); ok {
		if status.Platform == "" {
			status.Platform = platform
		}
		return status
	}
	if status, ok := existingReadyStatus(name, platform); ok {
		return status
	}
	if status, ok := m.storedStatus(name); ok {
		return status
	}
	return Status{
		Tool:     name,
		Platform: platform,
		Path:     ExecutablePath(m.root, name),
		State:    StateMissing,
	}
}

func (m *Manager) BinDir(name string, env map[string]string) string {
	status := m.Status(name, env)
	if status.State != StateReady || strings.TrimSpace(status.Path) == "" {
		return ""
	}
	return filepath.Dir(status.Path)
}

func (m *Manager) localReadyStatus(name string) (Status, bool) {
	path := ExecutablePath(m.root, name)
	if !fileExists(path) {
		return Status{}, false
	}
	platform, _ := platformKey(runtime.GOOS, runtime.GOARCH)
	return Status{
		Tool:     name,
		Platform: platform,
		Path:     path,
		State:    StateReady,
	}, true
}

func existingReadyStatus(name, platform string) (Status, bool) {
	if name != AntigravityCLI {
		return Status{}, false
	}
	path, err := execpath.Resolve(ExecutableName(name))
	if err != nil {
		return Status{}, false
	}
	return Status{
		Tool:     name,
		Platform: platform,
		Path:     path,
		State:    StateReady,
	}, true
}

func (m *Manager) storedStatus(name string) (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.status[name]
	return status, ok
}

func (m *Manager) setStatus(name string, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status[name] = status
}

func (m *Manager) setResolveErrorStatus(name string, err error) {
	platform, platformErr := platformKey(runtime.GOOS, runtime.GOARCH)
	state := StateFailed
	if platformErr != nil {
		state = StateUnsupported
	}
	m.setStatus(name, Status{
		Tool:     name,
		Platform: platform,
		State:    state,
		Message:  err.Error(),
	})
}

func displayName(name string) string {
	switch name {
	case AntigravityCLI:
		return "Antigravity CLI"
	case MuseCLI:
		return "Muse Code"
	default:
		return name
	}
}
