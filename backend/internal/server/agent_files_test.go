package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentFilesIgnoreHeartbeat(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "agents")
	writeFile(t, root, "SOUL.md", "soul")
	writeFile(t, root, "HEARTBEAT.md", "heartbeat")

	handler := (&Server{Root: root}).Handler()
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, httptest.NewRequest(http.MethodGet, "/v1/agent/files", nil))
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Files []agentFile `json:"files"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Files) != 2 || listed.Files[0].Name != "AGENTS.md" || listed.Files[1].Name != "SOUL.md" {
		t.Fatalf("files = %#v, want only AGENTS.md and SOUL.md", listed.Files)
	}

	writeRes := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/agent/files/HEARTBEAT.md", strings.NewReader(`{"content":"new"}`))
	handler.ServeHTTP(writeRes, req)
	if writeRes.Code != http.StatusBadRequest {
		t.Fatalf("write status = %d, want 400; body = %s", writeRes.Code, writeRes.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "HEARTBEAT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "heartbeat" {
		t.Fatalf("HEARTBEAT.md was modified: %q", data)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
