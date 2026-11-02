package acpadapter

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

// .github/acp-adapter-assets.json is the single source of truth for managed
// adapter versions; the backend embeds the manifest generated from it. These
// tests check that manifest: every built-in managed adapter has a complete,
// safe pin.

func TestBuiltinManagedAdaptersArePinned(t *testing.T) {
	pinned := parsePinnedManifest()
	for name, cfg := range acp.BuiltinAgents() {
		if cfg.ManagedAdapter == "" {
			continue
		}
		if len(pinned.Adapters[cfg.ManagedAdapter].Assets) == 0 {
			t.Errorf("agent %q uses managed adapter %q with no pinned assets", name, cfg.ManagedAdapter)
		}
	}
}

func TestPinnedAdapterAssetsAreComplete(t *testing.T) {
	for name, adapter := range parsePinnedManifest().Adapters {
		if !cleanRelative(adapter.Version) {
			t.Errorf("adapter %q: invalid version %q", name, adapter.Version)
		}
		for platform, asset := range adapter.Assets {
			if asset.URL == "" || len(asset.SHA256) != 64 || !cleanRelative(asset.Binary) {
				t.Errorf("adapter %q %s: incomplete asset %#v", name, platform, asset)
			}
			for key, path := range asset.Env {
				if key == "" || !cleanRelative(path) {
					t.Errorf("adapter %q %s: invalid environment path %q=%q", name, platform, key, path)
				}
			}
		}
	}
}

func TestCodexAssetsDeclareRuntime(t *testing.T) {
	codex := parsePinnedManifest().Adapters["codex"]
	for platform, asset := range codex.Assets {
		triple := map[string]string{
			"darwin-arm64": "aarch64-apple-darwin",
			"darwin-x64":   "x86_64-apple-darwin",
			"linux-arm64":  "aarch64-unknown-linux-musl",
			"linux-x64":    "x86_64-unknown-linux-musl",
			"win32-arm64":  "aarch64-pc-windows-msvc",
			"win32-x64":    "x86_64-pc-windows-msvc",
		}[platform]
		if triple == "" {
			t.Errorf("codex has unexpected platform %q", platform)
			continue
		}
		executable := ""
		if strings.HasPrefix(platform, "win32-") {
			executable = ".exe"
		}
		root := "codex-runtime/" + triple + "/bin/"
		if got, want := asset.Env["CODEX_PATH"], root+"codex"+executable; got != want {
			t.Errorf("codex %s CODEX_PATH = %q, want %q", platform, got, want)
		}
		if got, want := asset.Env["CODEX_CODE_MODE_HOST_PATH"], root+"codex-code-mode-host"+executable; got != want {
			t.Errorf("codex %s host = %q, want %q", platform, got, want)
		}
	}
}
