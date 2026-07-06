package managedtool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type toolSpec struct {
	Tool     string
	Version  string
	Platform string
	URL      string
	SHA512   string
	Root     string
	Command  string
}

type antigravityManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA512  string `json:"sha512"`
}

func (m *Manager) resolveSpec(ctx context.Context, name string) (toolSpec, error) {
	if name != AntigravityCLI {
		return toolSpec{}, fmt.Errorf("managed tool %q is not supported", name)
	}
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return toolSpec{}, err
	}
	manifest, err := m.fetchAntigravityManifest(ctx, platform)
	if err != nil {
		return toolSpec{}, err
	}
	if strings.TrimSpace(manifest.Version) == "" || strings.TrimSpace(manifest.URL) == "" || strings.TrimSpace(manifest.SHA512) == "" {
		return toolSpec{}, fmt.Errorf("Antigravity CLI manifest is incomplete")
	}
	root := filepath.Join(m.root, "acp", "managed", "tools", name, platform)
	return toolSpec{
		Tool:     name,
		Version:  strings.TrimSpace(manifest.Version),
		Platform: platform,
		URL:      strings.TrimSpace(manifest.URL),
		SHA512:   strings.TrimSpace(manifest.SHA512),
		Root:     root,
		Command:  filepath.Join(root, ExecutableName(name)),
	}, nil
}

func (m *Manager) fetchAntigravityManifest(ctx context.Context, platform string) (antigravityManifest, error) {
	base := strings.TrimRight(strings.TrimSpace(m.baseURL), "/")
	if base == "" {
		return antigravityManifest{}, fmt.Errorf("Antigravity CLI manifest URL is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/manifests/"+platform+".json", nil)
	if err != nil {
		return antigravityManifest{}, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return antigravityManifest{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return antigravityManifest{}, fmt.Errorf("fetch Antigravity CLI manifest: %s", res.Status)
	}
	var out antigravityManifest
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return antigravityManifest{}, err
	}
	return out, nil
}

func platformKey(goos, goarch string) (string, error) {
	arch := ""
	switch goarch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported managed tool architecture %s", goarch)
	}
	switch goos {
	case "darwin":
		return "darwin_" + arch, nil
	case "linux":
		if linuxMusl() {
			return "linux_" + arch + "_musl", nil
		}
		return "linux_" + arch, nil
	case "windows":
		return "windows_" + arch, nil
	default:
		return "", fmt.Errorf("unsupported managed tool OS %s", goos)
	}
}

func linuxMusl() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	for _, path := range []string{"/lib/libc.musl-x86_64.so.1", "/lib/libc.musl-aarch64.so.1"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	out, err := exec.Command("ldd", "/bin/ls").CombinedOutput()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "musl")
}

func ExecutableName(name string) string {
	switch name {
	case AntigravityCLI:
		if runtime.GOOS == "windows" {
			return "agy.exe"
		}
		return "agy"
	default:
		return name
	}
}

func ExecutablePath(root, name string) string {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return ""
	}
	return filepath.Join(root, "acp", "managed", "tools", strings.TrimSpace(name), platform, ExecutableName(name))
}

func BinDir(root, name string) string {
	path := ExecutablePath(root, name)
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Dir(path)
}
