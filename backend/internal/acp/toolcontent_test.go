package acp

import (
	"encoding/json"
	"strings"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func rawContent(t *testing.T, items ...string) []acpschema.ToolCallContent {
	t.Helper()
	out := make([]acpschema.ToolCallContent, 0, len(items))
	for _, item := range items {
		out = append(out, acpschema.ToolCallContent(json.RawMessage(item)))
	}
	return out
}

func TestNormalizeToolContentWebSearchResults(t *testing.T) {
	// Claude forwards each web_search_result as a text content block "Title (url)".
	content := rawContent(t,
		`{"type":"content","content":{"type":"text","text":"Stanford AI Index 2025 (https://hai.stanford.edu/ai-index)"}}`,
		`{"type":"content","content":{"type":"text","text":"AI adoption by country (https://www.visualcapitalist.com/x)"}}`,
	)
	got := normalizeToolContent(content)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got))
	}
	if got[0].Type != "text" || !strings.Contains(got[0].Text, "hai.stanford.edu") {
		t.Fatalf("unexpected first block: %+v", got[0])
	}
}

func TestNormalizeToolContentLinkAndDiff(t *testing.T) {
	content := rawContent(t,
		`{"type":"content","content":{"type":"resource_link","uri":"https://example.com/doc","name":"Doc","mimeType":"text/html"}}`,
		`{"type":"diff","path":"main.go","oldText":"a","newText":"b"}`,
		`{"type":"content","content":{"type":"image","data":"...","mimeType":"image/png"}}`,
	)
	got := normalizeToolContent(content)
	if len(got) != 2 {
		t.Fatalf("expected image block dropped, got %d blocks: %+v", len(got), got)
	}
	if got[0].Type != "link" || got[0].URI != "https://example.com/doc" || got[0].Title != "Doc" {
		t.Fatalf("unexpected link block: %+v", got[0])
	}
	if got[1].Type != "diff" || got[1].Path != "main.go" || got[1].NewText != "b" {
		t.Fatalf("unexpected diff block: %+v", got[1])
	}
}

func TestToolUpdateMergeSemantics(t *testing.T) {
	kind := acpschema.ToolKindFetch
	completed := acpschema.ToolCallStatusCompleted
	call := ToolCallSnapshot{ID: "t1", Title: "\"query\"", Status: "completed"}

	// First update carries everything.
	mergeToolCall(&call, toolUpdateSnapshot("t1", "\"query\"", &completed, &kind,
		rawContent(t, `{"type":"content","content":{"type":"text","text":"Result (https://x.com)"}}`),
		json.RawMessage(`{"query":"q"}`),
		map[string]any{"claudeCode": map[string]any{"toolName": "WebSearch"}},
	))
	if call.Kind != "fetch" || call.ToolName != "WebSearch" {
		t.Fatalf("kind/toolName not captured: %+v", call)
	}
	if len(call.Content) != 1 || len(call.RawInput) == 0 {
		t.Fatalf("rich fields not captured: %+v", call)
	}

	// A sparse follow-up (id only) must NOT clear the captured content/kind.
	mergeToolCall(&call, toolUpdateSnapshot("t1", "", nil, nil, nil, nil, nil))
	if call.Kind != "fetch" || call.ToolName != "WebSearch" || len(call.Content) != 1 {
		t.Fatalf("sparse update wrongly cleared fields: %+v", call)
	}
}

func TestBoundedRawInput(t *testing.T) {
	big := json.RawMessage(`{"q":"` + strings.Repeat("x", maxToolRawInputBytes) + `"}`)
	if boundedRawInput(big) != nil {
		t.Fatalf("oversized rawInput should be dropped")
	}
	if boundedRawInput(json.RawMessage(`{not json`)) != nil {
		t.Fatalf("invalid rawInput should be dropped")
	}
	if boundedRawInput(json.RawMessage(`{"url":"https://x.com"}`)) == nil {
		t.Fatalf("small valid rawInput should be kept")
	}
}

func TestClampToolTextRuneSafe(t *testing.T) {
	long := strings.Repeat("é", maxToolContentText+50)
	got := clampToolText(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix")
	}
	if []rune(got)[0] != 'é' {
		t.Fatalf("expected valid leading rune")
	}
}
