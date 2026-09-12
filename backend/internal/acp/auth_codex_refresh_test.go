package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexRefreshUsesAccountRPCOnly(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	name := "codex"
	if strings.HasSuffix(executable, ".exe") {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAZ_TEST_CODEX_REFRESH", dir)
	t.Setenv("OPENAI_API_KEY", "must-not-reach-codex")
	for _, managed := range []bool{false, true} {
		managerConfig := Config{}
		cfg := AgentConfig{LoginBinDir: dir}
		if managed {
			managerConfig.Adapters = refreshAdapter(filepath.Join(dir, name))
			cfg = AgentConfig{ManagedAdapter: "codex"}
		}
		manager := NewManager(nil, managerConfig, nil)
		if err := manager.RefreshCodexOAuth(context.Background(), cfg, dir); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(filepath.Join(dir, "account-read"))
		if err != nil || string(data) != "refreshed" {
			t.Fatalf("native account refresh was not completed: %q, %v", data, err)
		}
		if err := os.Remove(filepath.Join(dir, "account-read")); err != nil {
			t.Fatal(err)
		}
	}
}

type refreshAdapter string

func (r refreshAdapter) ResolveAdapter(context.Context, string) (AdapterLaunch, error) {
	return AdapterLaunch{Env: map[string]string{"CODEX_PATH": string(r)}}, nil
}

func TestMain(m *testing.M) {
	dir := os.Getenv("JAZ_TEST_CODEX_REFRESH")
	if dir == "" {
		os.Exit(m.Run())
	}
	if len(os.Args) != 2 || os.Args[1] != "app-server" {
		os.Exit(7)
	}
	if os.Getenv("CODEX_HOME") != dir || os.Getenv("OPENAI_API_KEY") != "" {
		os.Exit(2)
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for _, method := range []string{"initialize", "initialized", "account/read"} {
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if decoder.Decode(&request) != nil || request.Method != method {
			os.Exit(3)
		}
		if method == "account/read" {
			var params struct {
				Refresh bool `json:"refreshToken"`
			}
			if json.Unmarshal(request.Params, &params) != nil || !params.Refresh {
				os.Exit(4)
			}
			if os.WriteFile(filepath.Join(dir, "account-read"), []byte("refreshed"), 0600) != nil {
				os.Exit(5)
			}
		}
		if request.ID != 0 {
			_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{}})
		}
	}
	var extra json.RawMessage
	if decoder.Decode(&extra) == nil {
		os.Exit(6)
	}
	os.Exit(0)
}
