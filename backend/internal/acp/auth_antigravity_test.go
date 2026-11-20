package acp_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/acp"
)

func TestAntigravityDisconnectUsesNativeLogout(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "antigravity"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	body, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, name), body, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		logout  string
		managed bool
	}{
		{name: "native command", logout: "1"},
		{name: "managed native command", logout: "1", managed: true},
		{name: "unsupported"},
		{name: "provider error", logout: "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			requestsPath := filepath.Join(root, "requests.jsonl")
			credential := filepath.Join(root, ".gemini", "antigravity-cli", "antigravity-oauth-token")
			if err := os.MkdirAll(filepath.Dir(credential), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(credential, []byte("provider-owned"), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := acp.AgentConfig{
				Command: executable,
				Args:    []string{"-test.run=TestFakeACPAgentProcess"},
				Env: map[string]string{
					"JAZ_FAKE_ACP_AGENT":       "1",
					"JAZ_FAKE_ACP_LOGOUT":      test.logout,
					"JAZ_FAKE_ACP_REQUEST_LOG": requestsPath,
				},
			}
			if test.managed {
				cfg.ManagedAdapter = acp.AgentAntigravity
				cfg.ManagedAdapterArgs = cfg.Args
				cfg.Command = ""
				cfg.Args = nil
				cfg.LoginBinDir = bin
			}
			err := acp.DisconnectAgentAuth(t.Context(), acp.AgentAntigravity, cfg, root, nil)
			wantMethods := []string{"initialize", "logout"}
			switch test.logout {
			case "":
				if err == nil || !strings.Contains(err.Error(), "does not advertise native sign out") {
					t.Fatalf("unsupported native logout error = %v", err)
				}
				wantMethods = wantMethods[:1]
			case "error":
				var rpcErr *jsonrpc.Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != -32005 || rpcErr.Message != "provider refused sign out" {
					t.Fatalf("native logout error lost provider details: %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			var methods []string
			for _, request := range readFakeACPRequests(t, requestsPath) {
				methods = append(methods, request.Method)
			}
			if !slices.Equal(methods, wantMethods) {
				t.Fatalf("native sign out requests = %v, want %v", methods, wantMethods)
			}
			if data, err := os.ReadFile(credential); err != nil || string(data) != "provider-owned" {
				t.Fatalf("Jaz changed provider credential storage: %q, %v", data, err)
			}
		})
	}
}
