package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jazmem/pkg/jazmem"
)

func TestSearchToolUsesAgenticJazmemSearch(t *testing.T) {
	llm := fakeProvider(t, `{"answer":"Alice works on jazmem MCP testing.","citation_ids":[1],"gaps":[],"warnings":[]}`)
	defer llm.Close()
	mem := testMemory(t, jazmem.Config{
		APIKey:           "test-key",
		ProviderEndpoint: llm.URL,
		Model:            "test-model",
	})
	defer mem.Close()
	writePage(t, mem, "people/alice-bentick", "---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n\nAlice works on jazmem MCP testing.\n")
	if _, err := mem.Reindex(context.Background(), jazmem.ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	result, err := (&SearchTool{Memory: mem}).Execute(context.Background(), map[string]any{"query": "Alice jazmem"})
	if err != nil {
		t.Fatal(err)
	}
	var payload jazmem.AgenticResponse
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode search result: %v\n%s", err, result.Content)
	}
	if payload.Answer == "" || payload.ModelUsed != "test-model" || len(payload.Citations) == 0 || payload.Citations[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected agentic payload %#v", payload)
	}
}

func TestGetToolReturnsRawMarkdownAndSuggestions(t *testing.T) {
	mem := testMemory(t, jazmem.Config{})
	defer mem.Close()
	writePage(t, mem, "people/alice-bentick", "---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n\nAlice works on jazmem MCP testing.\n")

	tool := &GetTool{Memory: mem}
	result, err := tool.Execute(context.Background(), map[string]any{"slug": "people/alice-bentick"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Content, "---\ntitle: Alice Bentick") || !strings.Contains(result.Content, "Alice works on jazmem") {
		t.Fatalf("expected raw markdown, got %q", result.Content)
	}

	missing, err := tool.Execute(context.Background(), map[string]any{"slug": "people/alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(missing.Content, "not found: people/alice") || !strings.Contains(missing.Content, "people/alice-bentick") {
		t.Fatalf("unexpected missing output %q", missing.Content)
	}
}

func testMemory(t *testing.T, cfg jazmem.Config) *jazmem.Memory {
	t.Helper()
	cfg.Root = t.TempDir()
	cfg.DBPath = filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := jazmem.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return mem
}

func writePage(t *testing.T, mem *jazmem.Memory, slug, content string) {
	t.Helper()
	path := filepath.Join(mem.Root(), filepath.FromSlash(slug)+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeProvider(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected provider path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}
