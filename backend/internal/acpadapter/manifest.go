package acpadapter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// pinnedManifest is generated from .github/acp-adapter-assets.json, so each
// build carries the adapter versions it was released with.
//
//go:embed acp-adapters.json
var pinnedManifest []byte

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

func parsePinnedManifest() manifest {
	var out manifest
	if err := json.Unmarshal(pinnedManifest, &out); err != nil {
		panic(fmt.Sprintf("parse embedded acp adapter manifest: %v", err))
	}
	return out
}

func (m *Manager) resolveSpec(name string) (adapterSpec, error) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return adapterSpec{}, err
	}
	return m.specFromManifest(name, platform)
}

func (m *Manager) specFromManifest(name, platform string) (adapterSpec, error) {
	adapter, ok := m.manifest.Adapters[name]
	if !ok {
		return adapterSpec{}, fmt.Errorf("managed acp adapter %q is not in the manifest", name)
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
	return value != "" && value != "." && path.Clean(value) == value && !path.IsAbs(value) && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, `:\`)
}

func resolveArchivePath(root, value string) string {
	return filepath.Join(root, filepath.FromSlash(strings.TrimSpace(value)))
}
