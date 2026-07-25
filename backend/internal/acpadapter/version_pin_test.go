package acpadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

type adapterAssetSpec struct {
	Adapters map[string]struct {
		Tag    string `json:"tag"`
		Assets map[string]struct {
			Binary string            `json:"binary"`
			Env    map[string]string `json:"env"`
		} `json:"assets"`
	} `json:"adapters"`
}

// .github/acp-adapter-assets.json is the single source of truth for managed
// adapter versions: the backend carries no adapter version in code. These tests
// keep it that way and ensure every built-in managed adapter has a complete pin.

func TestBuiltinManagedAdaptersArePinned(t *testing.T) {
	spec := loadAdapterAssetSpec(t)
	for name, cfg := range acp.BuiltinAgents() {
		adapter := strings.TrimSpace(cfg.ManagedAdapter)
		if adapter == "" {
			continue
		}
		entry, ok := spec.Adapters[adapter]
		if !ok {
			t.Fatalf("agent %q uses managed adapter %q with no entry in acp-adapter-assets.json", name, adapter)
		}
		if entry.Tag == "" || len(entry.Assets) == 0 {
			t.Fatalf("managed adapter %q is missing tag/assets in acp-adapter-assets.json", adapter)
		}
	}
}

func TestAdapterAssetsAreComplete(t *testing.T) {
	spec := loadAdapterAssetSpec(t)
	for name, entry := range spec.Adapters {
		for platform, asset := range entry.Assets {
			if strings.TrimSpace(asset.Binary) == "" {
				t.Errorf("adapter %q %s: missing binary name", name, platform)
			}
			for key, path := range asset.Env {
				if strings.TrimSpace(key) == "" || strings.TrimSpace(path) == "" {
					t.Errorf("adapter %q %s: incomplete environment path", name, platform)
				}
			}
		}
	}
}

func TestCodexAssetsDeclareCodeModeHost(t *testing.T) {
	codex := loadAdapterAssetSpec(t).Adapters["codex"]
	for platform, asset := range codex.Assets {
		want := "codex-code-mode-host"
		if strings.HasPrefix(platform, "win32-") {
			want += ".exe"
		}
		if got := asset.Env["CODEX_CODE_MODE_HOST_PATH"]; got != want {
			t.Errorf("codex %s host = %q, want %q", platform, got, want)
		}
	}
}

func TestCodexAppServerAssetsDeclareRuntime(t *testing.T) {
	codex := loadAdapterAssetSpec(t).Adapters["codex-app-server"]
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
			t.Errorf("codex-app-server has unexpected platform %q", platform)
			continue
		}
		executable := ""
		if strings.HasPrefix(platform, "win32-") {
			executable = ".exe"
		}
		root := "codex-runtime/" + triple + "/bin/"
		if got, want := asset.Env["CODEX_PATH"], root+"codex"+executable; got != want {
			t.Errorf("codex-app-server %s CODEX_PATH = %q, want %q", platform, got, want)
		}
		if got, want := asset.Env["CODEX_CODE_MODE_HOST_PATH"], root+"codex-code-mode-host"+executable; got != want {
			t.Errorf("codex-app-server %s host = %q, want %q", platform, got, want)
		}
	}
}

func loadAdapterAssetSpec(t *testing.T) adapterAssetSpec {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", ".github", "acp-adapter-assets.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read adapter asset spec: %v", err)
	}
	var spec adapterAssetSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse adapter asset spec: %v", err)
	}
	if len(spec.Adapters) == 0 {
		t.Fatal("adapter asset spec has no adapters")
	}
	return spec
}
