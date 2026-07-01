package acpadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const releasesURL = "https://github.com/gluonfield/jaz/releases"

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

type managedAdapterAssetSpec struct {
	Adapters map[string]managedAdapterAssetSpecEntry `json:"adapters"`
}

type managedAdapterAssetSpecEntry struct {
	Repo    string                                  `json:"repo"`
	Tag     string                                  `json:"tag"`
	Version string                                  `json:"version"`
	Assets  map[string]managedAdapterAssetSpecAsset `json:"assets"`
}

type managedAdapterAssetSpecAsset struct {
	Name   string            `json:"name"`
	Binary string            `json:"binary"`
	Env    map[string]string `json:"env,omitempty"`
}

type githubRelease struct {
	Assets []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func manifestURLForVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return releasesURL + "/latest/download/acp-adapters.json"
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return releasesURL + "/download/" + version + "/acp-adapters.json"
}

func manifestCacheNameForVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return "latest"
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func usesLatestManifest(version string) bool {
	version = strings.TrimSpace(version)
	return version == "" || version == "dev"
}

func (m *Manager) resolveSpec(ctx context.Context, name string) (adapterSpec, error) {
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return adapterSpec{}, err
	}
	manifest, err := m.fetchManifest(ctx)
	if err != nil {
		return adapterSpec{}, err
	}
	adapter, ok := manifest.Adapters[name]
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

func (m *Manager) fetchManifest(ctx context.Context) (manifest, error) {
	out, err := m.fetchManifestSource(ctx)
	if err == nil {
		m.cacheManifest(out)
		_ = m.writeManifestCache(out)
		return out, nil
	}
	if cached, ok := m.cachedManifest(); ok {
		if !m.cacheAllowedForFetchFailure(cached) {
			return manifest{}, err
		}
		return cached, nil
	}
	if cached, ok := m.readManifestCache(); ok {
		if !m.cacheAllowedForFetchFailure(cached) {
			return manifest{}, err
		}
		m.cacheManifest(cached)
		return cached, nil
	}
	return manifest{}, err
}

func (m *Manager) cacheAllowedForFetchFailure(cached manifest) bool {
	if m.assetSpecPath == "" {
		return true
	}
	spec, err := m.readAssetSpec()
	if err != nil {
		return false
	}
	for name, pinned := range spec.Adapters {
		adapter, ok := cached.Adapters[name]
		if !ok || adapter.Version != pinned.Version {
			return false
		}
		for platform, pinnedAsset := range pinned.Assets {
			asset, ok := adapter.Assets[platform]
			if !ok || !manifestAssetMatchesSpec(name, adapter.Version, asset, pinnedAsset) {
				return false
			}
		}
	}
	return true
}

func manifestAssetMatchesSpec(adapter, version string, asset manifestAsset, pinned managedAdapterAssetSpecAsset) bool {
	if err := validateManifestAsset(adapter, version, asset); err != nil {
		return false
	}
	if asset.Binary != strings.TrimSpace(pinned.Binary) {
		return false
	}
	if !maps.Equal(asset.Env, pinned.Env) {
		return false
	}
	parsed, err := url.Parse(asset.URL)
	return err == nil && path.Base(parsed.Path) == strings.TrimSpace(pinned.Name)
}

func (m *Manager) fetchManifestSource(ctx context.Context) (manifest, error) {
	if m.assetSpecPath != "" {
		return m.manifestFromAssetSpec(ctx)
	}
	if m.manifestURL == "" {
		return manifest{}, fmt.Errorf("managed acp adapter manifest URL is not configured")
	}
	return m.fetchRemoteManifest(ctx)
}

func (m *Manager) manifestFromAssetSpec(ctx context.Context) (manifest, error) {
	spec, err := m.readAssetSpec()
	if err != nil {
		return manifest{}, err
	}
	out := manifest{Adapters: map[string]manifestAdapter{}}
	for name, adapter := range spec.Adapters {
		releaseAssets, err := m.fetchReleaseAssets(ctx, adapter.Repo, adapter.Tag)
		if err != nil {
			return manifest{}, err
		}
		out.Adapters[name] = manifestAdapter{Version: adapter.Version, Assets: map[string]manifestAsset{}}
		for platform, wanted := range adapter.Assets {
			asset, ok := releaseAssets[wanted.Name]
			if !ok {
				return manifest{}, fmt.Errorf("%s@%s is missing %s", adapter.Repo, adapter.Tag, wanted.Name)
			}
			sha256 := strings.TrimPrefix(asset.Digest, "sha256:")
			if sha256 == asset.Digest || sha256 == "" {
				return manifest{}, fmt.Errorf("%s is missing a GitHub SHA-256 digest", asset.Name)
			}
			out.Adapters[name].Assets[platform] = manifestAsset{
				URL:    asset.BrowserDownloadURL,
				SHA256: sha256,
				Binary: wanted.Binary,
				Env:    wanted.Env,
			}
		}
	}
	return out, nil
}

func (m *Manager) readAssetSpec() (managedAdapterAssetSpec, error) {
	body, err := os.ReadFile(m.assetSpecPath)
	if err != nil {
		return managedAdapterAssetSpec{}, err
	}
	var spec managedAdapterAssetSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return managedAdapterAssetSpec{}, err
	}
	if len(spec.Adapters) == 0 {
		return managedAdapterAssetSpec{}, fmt.Errorf("managed acp adapter asset spec is empty")
	}
	for name, adapter := range spec.Adapters {
		if err := validateAdapterAssetSpec(name, adapter); err != nil {
			return managedAdapterAssetSpec{}, err
		}
	}
	return spec, nil
}

func (m *Manager) fetchReleaseAssets(ctx context.Context, repo, tag string) (map[string]githubReleaseAsset, error) {
	endpoint := strings.TrimRight(m.githubAPIURL, "/") + "/" + strings.Trim(repo, "/") + "/releases/tags/" + url.PathEscape(strings.TrimSpace(tag))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s@%s release assets: %s", repo, tag, res.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		return nil, err
	}
	out := make(map[string]githubReleaseAsset, len(release.Assets))
	for _, asset := range release.Assets {
		out[asset.Name] = asset
	}
	return out, nil
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
	return filepath.Join(m.root, "acp", "managed", "adapters", "manifest-"+m.manifestCacheName+".json")
}

func findLocalAssetSpecPath() string {
	dir, err := os.Getwd()
	if err == nil {
		if path := findAssetSpecFromDir(dir); path != "" {
			return path
		}
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return findAssetSpecFromDir(filepath.Dir(file))
	}
	return ""
}

func findAssetSpecFromDir(dir string) string {
	for {
		candidate := filepath.Join(dir, ".github", "acp-adapter-assets.json")
		if fileExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
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

func validateAdapterAssetSpec(name string, adapter managedAdapterAssetSpecEntry) error {
	tag := strings.TrimSpace(adapter.Tag)
	version := strings.TrimSpace(adapter.Version)
	if strings.TrimSpace(adapter.Repo) == "" || tag == "" || version == "" {
		return fmt.Errorf("managed acp adapter %q asset spec is incomplete", name)
	}
	if tag != version && tag != "v"+version {
		return fmt.Errorf("%s: tag %q does not match version %q", name, tag, version)
	}
	if len(adapter.Assets) == 0 {
		return fmt.Errorf("managed acp adapter %q asset spec is incomplete", name)
	}
	for platform, asset := range adapter.Assets {
		if strings.TrimSpace(asset.Name) == "" || strings.TrimSpace(asset.Binary) == "" {
			return fmt.Errorf("managed acp adapter %q %s asset spec is incomplete", name, platform)
		}
		if !strings.Contains(asset.Name, version) {
			return fmt.Errorf("%s %s: asset %q does not embed version %q", name, platform, asset.Name, version)
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
