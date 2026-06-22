package acp

import (
	"encoding/json"
	"unicode/utf8"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	maxToolContentBlocks = 64
	maxToolContentText   = 8000
	maxToolRawInputBytes = 2048
)

// mergeToolCall overlays the populated fields of src onto dst. ACP updates (and
// native local-agent updates) are sparse — only changed fields are present — so
// an empty field never clears what an earlier update already established. This is
// the single merge rule shared by the external-ACP and native paths.
func mergeToolCall(dst *ToolCallSnapshot, src ToolCallSnapshot) {
	if src.ID != "" {
		dst.ID = src.ID
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.Status != "" {
		dst.Status = src.Status
	}
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.ToolName != "" {
		dst.ToolName = src.ToolName
	}
	if len(src.Content) > 0 {
		dst.Content = src.Content
	}
	if len(src.RawInput) > 0 {
		dst.RawInput = src.RawInput
	}
}

// toolUpdateSnapshot decodes one ACP tool-call update (a ToolCall or a
// ToolCallUpdate — same fields) into a partial snapshot. Both session-update
// variants share this so the protocol handler keeps a single code path; merge it
// onto the running call with mergeToolCall.
func toolUpdateSnapshot(id acpschema.ToolCallID, title string, status *acpschema.ToolCallStatus, kind *acpschema.ToolKind, content []acpschema.ToolCallContent, rawInput json.RawMessage, meta map[string]any) ToolCallSnapshot {
	src := ToolCallSnapshot{
		ID:       string(id),
		Title:    title,
		Kind:     kindString(kind),
		ToolName: metaToolName(meta),
		Content:  normalizeToolContent(content),
		RawInput: boundedRawInput(rawInput),
	}
	if status != nil {
		src.Status = string(*status)
	}
	return src
}

func kindString(kind *acpschema.ToolKind) string {
	if kind == nil {
		return ""
	}
	return string(*kind)
}

// boundedRawInput keeps small object inputs such as query/url/file path fields
// while dropping file-content-sized or non-object payloads.
func boundedRawInput(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || len(raw) > maxToolRawInputBytes {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

// metaToolName recovers the underlying tool name from the ACP _meta bag. Claude
// exposes it as _meta.claudeCode.toolName, which lets the UI tell WebSearch from
// WebFetch (both arrive as kind "fetch"). Agents that omit it fall back to kind.
func metaToolName(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if cc, ok := meta["claudeCode"].(map[string]any); ok {
		if name, ok := cc["toolName"].(string); ok {
			return name
		}
	}
	return ""
}

func normalizeToolContent(items []acpschema.ToolCallContent) []sessionevents.ACPToolContent {
	out := make([]sessionevents.ACPToolContent, 0, len(items))
	for _, raw := range items {
		if len(out) >= maxToolContentBlocks {
			break
		}
		if block := decodeToolContent(json.RawMessage(raw)); block != nil {
			out = append(out, *block)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decodeToolContent flattens one ACP ToolCallContent (a "content" block wrapping
// a ContentBlock, or a "diff") into the normalized display shape. Decoding stays
// field-driven rather than depending on every acp-transport sub-struct so any
// agent's blocks survive.
func decodeToolContent(raw json.RawMessage) *sessionevents.ACPToolContent {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	switch env.Type {
	case "content":
		var wrap struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil || len(wrap.Content) == 0 {
			return nil
		}
		return decodeContentBlock(wrap.Content)
	case "diff":
		var d struct {
			Path    string `json:"path"`
			NewText string `json:"newText"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil
		}
		return &sessionevents.ACPToolContent{
			Type:    "diff",
			Path:    d.Path,
			NewText: clampToolText(d.NewText),
		}
	default:
		return nil
	}
}

func decodeContentBlock(raw json.RawMessage) *sessionevents.ACPToolContent {
	var block struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		URI      string          `json:"uri"`
		Name     string          `json:"name"`
		Title    string          `json:"title"`
		Resource json.RawMessage `json:"resource"`
	}
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil
	}
	switch block.Type {
	case "text":
		text := clampToolText(block.Text)
		if text == "" {
			return nil
		}
		return &sessionevents.ACPToolContent{Type: "text", Text: text}
	case "resource_link":
		if block.URI == "" {
			return nil
		}
		return &sessionevents.ACPToolContent{
			Type:  "link",
			URI:   block.URI,
			Title: firstNonEmpty(block.Title, block.Name),
		}
	case "resource":
		var res struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		}
		if len(block.Resource) > 0 {
			_ = json.Unmarshal(block.Resource, &res)
		}
		if text := clampToolText(res.Text); text != "" {
			return &sessionevents.ACPToolContent{Type: "text", Text: text, URI: res.URI}
		}
		if res.URI != "" {
			return &sessionevents.ACPToolContent{Type: "link", URI: res.URI}
		}
		return nil
	default:
		// image / audio / unknown blocks carry no inline text worth showing.
		return nil
	}
}

func clampToolText(s string) string {
	if utf8.RuneCountInString(s) <= maxToolContentText {
		return s
	}
	return string([]rune(s)[:maxToolContentText]) + "…"
}
