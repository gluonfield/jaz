package acp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
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
	if got[1].Type != "diff" || got[1].Path != "main.go" || got[1].OldText != "a" || got[1].NewText != "b" {
		t.Fatalf("unexpected diff block: %+v", got[1])
	}
}

func TestToolUpdateMergeSemantics(t *testing.T) {
	kind := acpschema.ToolKindFetch
	completed := acpschema.ToolCallStatusCompleted
	call := sessionevents.ACPToolCall{ID: "t1", Title: "\"query\"", Status: "completed"}
	now := time.Now().UTC()

	// First update carries everything.
	mergeToolCall(&call, toolUpdateSnapshot(toolUpdateFields{
		ID:      "t1",
		Title:   "\"query\"",
		Status:  &completed,
		Kind:    &kind,
		Content: rawContent(t, `{"type":"content","content":{"type":"text","text":"Result (https://x.com)"}}`),
		Locations: []acpschema.ToolCallLocation{{
			Path: "results.json",
			Line: 3,
		}},
		RawInput:  json.RawMessage(`{"query":"q"}`),
		RawOutput: json.RawMessage(`{"content":"ok","authorization":"Bearer ya29.visible"}`),
		Meta:      map[string]any{"claudeCode": map[string]any{"toolName": "WebSearch"}},
		At:        now,
	}))
	if call.Kind != "fetch" || call.ToolName != "WebSearch" {
		t.Fatalf("kind/toolName not captured: %+v", call)
	}
	if len(call.Content) != 1 || len(call.RawInput) == 0 {
		t.Fatalf("rich fields not captured: %+v", call)
	}
	if len(call.Locations) != 1 || call.Locations[0].Path != "results.json" || call.Locations[0].Line != 3 {
		t.Fatalf("locations not captured: %+v", call.Locations)
	}
	if !strings.Contains(string(call.RawOutput), "[REDACTED]") || strings.Contains(string(call.RawOutput), "ya29.visible") {
		t.Fatalf("rawOutput not redacted: %s", call.RawOutput)
	}

	// A sparse follow-up (id only) must NOT clear the captured content/kind.
	mergeToolCall(&call, toolUpdateSnapshot(toolUpdateFields{ID: "t1", At: now}))
	if call.Kind != "fetch" || call.ToolName != "WebSearch" || len(call.Content) != 1 || len(call.Locations) != 1 || len(call.RawOutput) == 0 {
		t.Fatalf("sparse update wrongly cleared fields: %+v", call)
	}
}

func TestToolUpdateNormalizesCodexWebToolNames(t *testing.T) {
	fetch := acpschema.ToolKindFetch
	for _, test := range []struct {
		raw  string
		want string
	}{
		{`{"action":{"type":"search"}}`, "WebSearch"},
		{`{"action":{"type":"open_page"}}`, "WebFetch"},
		{`{"action":{"type":"find_in_page"}}`, "WebFetch"},
		{`{}`, "WebFetch"},
	} {
		call := toolUpdateSnapshot(toolUpdateFields{Kind: &fetch, RawInput: json.RawMessage(test.raw)})
		if call.ToolName != test.want {
			t.Fatalf("tool name for %s = %q, want %q", test.raw, call.ToolName, test.want)
		}
	}
	search := acpschema.ToolKindSearch
	call := toolUpdateSnapshot(toolUpdateFields{Kind: &search, RawInput: json.RawMessage(`{"action":{"type":"search"}}`)})
	if call.ToolName != "" {
		t.Fatalf("filesystem search tool name = %q, want empty", call.ToolName)
	}
}

func TestToolUpdateNormalizesProviderToolPresentation(t *testing.T) {
	call := toolUpdateSnapshot(toolUpdateFields{
		ID:       "search-1",
		Title:    "X Search",
		RawInput: json.RawMessage(`{"variant":"XSearch"}`),
		RawOutput: json.RawMessage(`{
			"action": {
				"query": "typed tool presentation",
				"url": "https://primary.example/result",
				"sources": [
					{"url":"https://one.example/result","title":"One"},
					{"url":"https://two.example/result","title":"Two"},
					{"url":"https://three.example/result","title":"Three"},
					{"url":"https://four.example/result","title":"Four"},
					{"url":"https://one.example/result","title":"Duplicate"}
				]
			}
		}`),
	})
	if call.ToolName != "WebSearch" || call.Title != "typed tool presentation" {
		t.Fatalf("normalized identity = %q %q", call.ToolName, call.Title)
	}
	if len(call.Content) != 5 {
		t.Fatalf("normalized result count = %d, want 5", len(call.Content))
	}
	if call.Content[1].Type != "link" || call.Content[1].URI != "https://one.example/result" || call.Content[1].Title != "One" {
		t.Fatalf("normalized result = %+v", call.Content[1])
	}

	for variant, want := range map[string]string{
		"Bash":     "Bash",
		"ListDir":  "LS",
		"ReadFile": "Read",
		"WebFetch": "WebFetch",
	} {
		got := toolUpdateSnapshot(toolUpdateFields{RawInput: json.RawMessage(`{"variant":"` + variant + `"}`)})
		if got.ToolName != want {
			t.Errorf("variant %q normalized to %q, want %q", variant, got.ToolName, want)
		}
	}
}

func TestToolUpdateCapturesRuntimeMetadata(t *testing.T) {
	now := time.Now().UTC()
	status := acpschema.ToolCallStatusInProgress
	call := toolUpdateSnapshot(toolUpdateFields{
		ID:     "bash-1",
		Title:  "Run tests",
		Status: &status,
		Meta: map[string]any{
			"terminal_info": map[string]any{"terminal_id": "term-1", "cwd": "/repo"},
			"claudeCode": map[string]any{
				"toolName":        "Bash",
				"parentToolUseId": "task-1",
				"toolResponse":    map[string]any{"elapsedTimeSeconds": 12.5},
			},
		},
		At: now,
	})
	if call.ToolName != "Bash" || call.Runtime.TerminalID != "term-1" || call.Runtime.TerminalCwd != "/repo" {
		t.Fatalf("runtime metadata not captured: %+v", call)
	}
	if call.Runtime.ParentToolUseID != "task-1" || call.Runtime.ElapsedTimeSeconds != 12.5 {
		t.Fatalf("claude metadata not captured: %+v", call.Runtime)
	}

	exit := toolUpdateSnapshot(toolUpdateFields{
		ID: "bash-1",
		Meta: map[string]any{
			"terminal_output": map[string]any{"terminal_id": "term-1", "data": "ok"},
			"terminal_exit":   map[string]any{"terminal_id": "term-1", "exit_code": 0},
		},
		At: now,
	})
	mergeToolCall(&call, exit)
	if call.Runtime.TerminalOutputAt.IsZero() {
		t.Fatalf("terminal output timestamp not captured: %+v", call.Runtime)
	}
	if call.Runtime.TerminalExitCode == nil || *call.Runtime.TerminalExitCode != 0 {
		t.Fatalf("terminal exit not captured: %+v", call.Runtime)
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
	if boundedRawInput(json.RawMessage(`["not","an","object"]`)) != nil {
		t.Fatalf("non-object rawInput should be dropped")
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

func TestClampToolTextRedactsOAuthTokens(t *testing.T) {
	input := `{"access_token":"ya29.visible","refresh_token":"1//visible","client_secret":"secret-visible"} Authorization: Bearer ya29.visible`
	got := clampToolText(input)
	for _, leaked := range []string{"ya29.visible", "1//visible", "secret-visible"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("tool text leaked %q in %q", leaked, got)
		}
	}
	if strings.Count(got, "[REDACTED]") != 4 {
		t.Fatalf("redacted text = %q", got)
	}
}

func TestToolContentRedactsOAuthTokensOutsideTextBlocks(t *testing.T) {
	content := rawContent(t,
		`{"type":"content","content":{"type":"resource_link","uri":"https://example.com/callback?token=ya29.visible","name":"Bearer 1//visible","mimeType":"text/html"}}`,
		`{"type":"diff","path":"token-1//visible.txt","oldText":"a","newText":"b"}`,
	)
	got := normalizeToolContent(content)
	if len(got) != 2 {
		t.Fatalf("content = %#v", got)
	}
	raw := boundedRawInput(json.RawMessage(`{"authorization":"Bearer ya29.visible","key-ya29.visible":"value","nested":{"refresh_token":"1//visible"},"items":["ya29.visible"]}`))
	serialized := mustMarshalString(t, map[string]any{"content": got, "raw": raw})
	for _, leaked := range []string{"ya29.visible", "1//visible"} {
		if strings.Contains(serialized, leaked) {
			t.Fatalf("normalized tool content leaked %q in %s", leaked, serialized)
		}
	}
}

func mustMarshalString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
