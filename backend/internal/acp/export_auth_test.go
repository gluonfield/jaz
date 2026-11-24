package acp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func TestExportLocalAuthFilesCodexClaudeAndKimi(t *testing.T) {
	root := t.TempDir()
	layout := runtimefiles.New(root)
	if err := os.MkdirAll(layout.ACPCodexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	codexAuth := `{"auth_mode":"chatgpt","tokens":{"access_token":"tok","refresh_token":"ref"}}`
	if err := os.WriteFile(filepath.Join(layout.ACPCodexHome, "auth.json"), []byte(codexAuth), 0o600); err != nil {
		t.Fatal(err)
	}

	gotCodex, err := ExportLocalAuthFiles(AgentCodex, AgentConfig{}, root, nil, nil)
	if err != nil {
		t.Fatalf("ExportLocalAuthFiles(codex): %v", err)
	}
	if string(gotCodex.Files["auth.json"]) != codexAuth {
		t.Fatalf("unexpected codex auth.json: %q", string(gotCodex.Files["auth.json"]))
	}
	if !gotCodex.OwnedByJaz {
		t.Fatal("expected Jaz-profile codex auth to be OwnedByJaz")
	}

	// Clearing owned credential removes it cleanly.
	if err := RemoveOwnedCredential(AgentCodex, gotCodex.StoragePath, root); err != nil {
		t.Fatalf("RemoveOwnedCredential: %v", err)
	}
	if _, err := os.Stat(gotCodex.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("expected owned auth.json to be removed, stat err=%v", err)
	}

	// External CLI auth is exported with OwnedByJaz == false and never deleted.
	extCodex := t.TempDir()
	if err := os.WriteFile(filepath.Join(extCodex, "auth.json"), []byte(codexAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", extCodex)
	gotExt, err := ExportLocalAuthFiles(AgentCodex, AgentConfig{}, root, nil, nil)
	if err != nil {
		t.Fatalf("ExportLocalAuthFiles(external codex): %v", err)
	}
	if gotExt.OwnedByJaz {
		t.Fatal("external CODEX_HOME must not be OwnedByJaz")
	}

	// Claude file-based credentials + .claude.json export.
	if err := os.MkdirAll(layout.ACPClaudeConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	claudeCreds := `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-xyz","refreshToken":"rt","expiresAt":1893456000000}}`
	claudeMeta := `{"oauthAccount":{"emailAddress":"dev@example.com"}}`
	if err := os.WriteFile(filepath.Join(layout.ACPClaudeConfig, ".credentials.json"), []byte(claudeCreds), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ACPClaudeConfig, ".claude.json"), []byte(claudeMeta), 0o600); err != nil {
		t.Fatal(err)
	}
	gotClaude, err := ExportLocalAuthFiles(AgentClaude, AgentConfig{}, root, nil, nil)
	if err != nil {
		t.Fatalf("ExportLocalAuthFiles(claude): %v", err)
	}
	if string(gotClaude.Files[".credentials.json"]) != claudeCreds || string(gotClaude.Files[".claude.json"]) != claudeMeta {
		t.Fatalf("unexpected claude export: %+v", gotClaude.Files)
	}

	// Kimi credentials export.
	kimiCredDir := filepath.Join(layout.ACPKimiHome, "credentials")
	if err := os.MkdirAll(kimiCredDir, 0o700); err != nil {
		t.Fatal(err)
	}
	kimiTok := `{"access_token":"kimi-tok"}`
	if err := os.WriteFile(filepath.Join(kimiCredDir, "kimi-code.json"), []byte(kimiTok), 0o600); err != nil {
		t.Fatal(err)
	}
	gotKimi, err := ExportLocalAuthFiles(AgentKimi, AgentConfig{}, root, nil, nil)
	if err != nil {
		t.Fatalf("ExportLocalAuthFiles(kimi): %v", err)
	}
	if string(gotKimi.Files["credentials/kimi-code.json"]) != kimiTok {
		t.Fatalf("unexpected kimi export: %+v", gotKimi.Files)
	}

	// Unsupported agent returns ErrUnsupportedCloudSyncAgent.
	if _, err := ExportLocalAuthFiles("unknown", AgentConfig{}, root, nil, nil); !errors.Is(err, ErrUnsupportedCloudSyncAgent) {
		t.Fatalf("expected ErrUnsupportedCloudSyncAgent, got %v", err)
	}
}
