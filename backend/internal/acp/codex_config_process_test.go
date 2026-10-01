package acp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexConfigProcessHelper(t *testing.T) {
	if os.Getenv("JAZ_CODEX_CONFIG_PROBE") != "1" {
		return
	}
	raw := os.Getenv("CODEX_CONFIG")
	if !strings.HasPrefix(raw, "@") {
		t.Fatal("expected config file reference")
	}
	path := strings.TrimPrefix(raw, "@")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	prompt, ok := config["developer_instructions"].(string)
	if !ok || fmt.Sprintf("%x", sha256.Sum256([]byte(prompt))) != os.Getenv("JAZ_CODEX_PROMPT_SHA") {
		t.Fatal("instructions changed")
	}
	if config["model_provider"] != "openai" || config["custom"] != "preserved" {
		t.Fatal("config changed")
	}
	if _, ok := config["model"]; ok {
		t.Fatal("native model selection overridden")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CODEX_HOME"), "auth.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("JAZ_CODEX_CONFIG_RECEIPT"), []byte(path), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestCodexConfigFileProcessLifecycle(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	manager := NewManager(nil, Config{Root: t.TempDir()}, nil)
	home := t.TempDir()
	auth := filepath.Join(home, "auth.json")
	if err := os.WriteFile(auth, []byte("test-credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, label := range []string{"first", "second"} {
		prompt := strings.Repeat(label+" Привет 🌍\n\"quoted\" \\path\n", 20000)
		receipt := filepath.Join(t.TempDir(), "receipt")
		env := map[string]string{"CODEX_HOME": home, "CODEX_CONFIG": `{"custom":"preserved"}`, "JAZ_CODEX_CONFIG_PROBE": "1", "JAZ_CODEX_PROMPT_SHA": fmt.Sprintf("%x", sha256.Sum256([]byte(prompt))), "JAZ_CODEX_CONFIG_RECEIPT": receipt}
		conn, stderr, err := manager.openConn(ctx, AgentCodex, AgentConfig{Command: executable, Args: []string{"-test.run=^TestCodexConfigProcessHelper$"}}, env, t.TempDir(), prompt)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		var path string
		for path == "" {
			if data, err := os.ReadFile(receipt); err == nil {
				path = string(data)
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("helper did not read config:", ctx.Err())
			case <-stderr.done:
				detail, err := stderr.snapshot()
				t.Fatalf("helper exited before read: %v %s", err, detail)
			case <-time.After(10 * time.Millisecond):
			}
		}
		paths = append(paths, path)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatal("config accessible by other users")
		}
		t.Cleanup(func() {
			_ = conn.Close()
			select {
			case <-stderr.done:
			case <-time.After(10 * time.Second):
				t.Error("helper did not stop")
				return
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("config remains after process exit: %v", err)
			}
		})
	}
	if paths[0] == paths[1] {
		t.Fatal("concurrent processes share a config")
	}
	data, err := os.ReadFile(auth)
	if err != nil || string(data) != "test-credential" {
		t.Fatal("auth changed")
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("shared config written")
	}
}

func TestCodexConfigFileRemovedOnStartFailure(t *testing.T) {
	manager := NewManager(nil, Config{Root: t.TempDir()}, nil)
	env := map[string]string{}
	_, _, err := manager.openConn(context.Background(), AgentCodex, AgentConfig{Command: filepath.Join(t.TempDir(), "missing-executable")}, env, t.TempDir(), "instructions")
	if err == nil {
		t.Fatal("missing executable started")
	}
	path := strings.TrimPrefix(env["CODEX_CONFIG"], "@")
	if path == "" {
		t.Fatal("config was not created")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config remains after failed launch: %v", err)
	}
}
