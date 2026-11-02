package acpadapter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
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
	adapter := m.manifest.Adapters[name]
	asset, ok := adapter.Assets[platform]
	if !ok {
		return adapterSpec{}, fmt.Errorf("managed acp adapter %q does not support %s", name, platform)
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
		spec.Env[key] = filepath.Join(root, filepath.FromSlash(value))
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
