package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/instructions"
)

func TestAgentFilesIgnoreHeartbeat(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "agents")
	writeFile(t, root, "SOUL.md", "soul")
	writeFile(t, root, "INTERNAL.md", "internal")
	writeFile(t, root, "HEARTBEAT.md", "heartbeat")

	handler := (&Server{Root: root, Instructions: instructions.NewLocalFSStore(root)}).Handler()
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
	if len(listed.Files) != 2 || listed.Files[0].Name != "AGENTS.md" || listed.Files[1].Name != "BOTS.md" {
		t.Fatalf("files = %#v, want only AGENTS.md and BOTS.md", listed.Files)
	}
	if listed.Files[0].Version == "" {
		t.Fatalf("expected AGENTS.md version digest, got %#v", listed.Files[0])
	}

	soulRes := httptest.NewRecorder()
	handler.ServeHTTP(soulRes, httptest.NewRequest(http.MethodPut, "/v1/agent/files/SOUL.md", strings.NewReader(`{"content":"new"}`)))
	if soulRes.Code != http.StatusBadRequest {
		t.Fatalf("SOUL.md write status = %d, want 400; body = %s", soulRes.Code, soulRes.Body.String())
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
	botRes := httptest.NewRecorder()
	handler.ServeHTTP(botRes, httptest.NewRequest(http.MethodPut, "/v1/agent/files/BOTS.md", strings.NewReader(`{"content":"Bot-only rules"}`)))
	if botRes.Code != http.StatusOK {
		t.Fatalf("save bot rules: %d, %s", botRes.Code, botRes.Body.String())
	}
	data, err = os.ReadFile(filepath.Join(root, "BOTS.md"))
	if err != nil || string(data) != "Bot-only rules" {
		t.Fatalf("saved bot rules = %q, %v", data, err)
	}

	// Matching If-Match succeeds and returns updated ETag/version; stale If-Match returns 409 without overwriting.
	agentsVer := listed.Files[0].Version
	okReq := httptest.NewRequest(http.MethodPut, "/v1/agent/files/AGENTS.md", strings.NewReader(`{"content":"agents v2"}`))
	okReq.Header.Set("If-Match", `"`+agentsVer+`"`)
	okRes := httptest.NewRecorder()
	handler.ServeHTTP(okRes, okReq)
	if okRes.Code != http.StatusOK || okRes.Header().Get("ETag") == "" {
		t.Fatalf("optimistic save failed: status=%d etag=%q body=%s", okRes.Code, okRes.Header().Get("ETag"), okRes.Body.String())
	}

	staleReq := httptest.NewRequest(http.MethodPut, "/v1/agent/files/AGENTS.md", strings.NewReader(`{"content":"stale overwrite"}`))
	staleReq.Header.Set("If-Match", `"`+agentsVer+`"`)
	staleRes := httptest.NewRecorder()
	handler.ServeHTTP(staleRes, staleReq)
	if staleRes.Code != http.StatusConflict {
		t.Fatalf("stale If-Match status = %d, want 409; body = %s", staleRes.Code, staleRes.Body.String())
	}
	staleBodyReq := httptest.NewRequest(http.MethodPut, "/v1/agent/files/AGENTS.md", strings.NewReader(`{"content":"stale body","version":"`+agentsVer+`"}`))
	staleBodyRes := httptest.NewRecorder()
	handler.ServeHTTP(staleBodyRes, staleBodyReq)
	if staleBodyRes.Code != http.StatusConflict {
		t.Fatalf("stale body version status = %d, want 409; body = %s", staleBodyRes.Code, staleBodyRes.Body.String())
	}
	data, err = os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil || string(data) != "agents v2" {
		t.Fatalf("stale writes modified AGENTS.md: %q, %v", data, err)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
