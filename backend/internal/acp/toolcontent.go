package acp

import (
	"encoding/json"
	"regexp"
	"time"
	"unicode/utf8"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	maxToolContentBlocks  = 64
	maxToolContentText    = 8000
	maxToolRawInputBytes  = 2048
	maxToolRawOutputBytes = 8192
)

var (
	oauthJSONFieldPattern = regexp.MustCompile(`(?i)("(?:(?:access|refresh|id)_token|client_secret)"\s*:\s*")[^"]+(")`)
	bearerTokenPattern    = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/\-]+=*`)
	googleAccessPattern   = regexp.MustCompile(`\bya29\.[A-Za-z0-9._\-]+`)
	googleRefreshPattern  = regexp.MustCompile(`\b1//[A-Za-z0-9._\-]+`)
)

// mergeToolCall overlays the populated fields of src onto dst. ACP updates (and
// native local-agent updates) are sparse — only changed fields are present — so
// an empty field never clears what an earlier update already established. This is
// the single merge rule shared by the external-ACP and native paths.
func mergeToolCall(dst *sessionevents.ACPToolCall, src sessionevents.ACPToolCall) {
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
	if len(src.Locations) > 0 {
		dst.Locations = src.Locations
	}
	if len(src.RawInput) > 0 {
		dst.RawInput = src.RawInput
	}
	if len(src.RawOutput) > 0 {
		dst.RawOutput = src.RawOutput
	}
	mergeACPToolRuntime(&dst.Runtime, src.Runtime)
	if !src.StartedAt.IsZero() && dst.StartedAt.IsZero() {
		dst.StartedAt = src.StartedAt
	}
	if !src.UpdatedAt.IsZero() {
		dst.UpdatedAt = src.UpdatedAt
	}
}

type toolUpdateFields struct {
	ID        acpschema.ToolCallID
	Title     string
	Status    *acpschema.ToolCallStatus
	Kind      *acpschema.ToolKind
	Content   []acpschema.ToolCallContent
	Locations []acpschema.ToolCallLocation
	RawInput  json.RawMessage
	RawOutput json.RawMessage
	Meta      map[string]any
	At        time.Time
}

// toolUpdateSnapshot decodes one ACP tool-call update (a ToolCall or
// ToolCallUpdate) into a partial snapshot. Both session-update variants share
// this so the protocol handler keeps one merge path.
func toolUpdateSnapshot(fields toolUpdateFields) sessionevents.ACPToolCall {
	src := sessionevents.ACPToolCall{
		ID:        string(fields.ID),
		Title:     fields.Title,
		Kind:      kindString(fields.Kind),
		ToolName:  normalizedToolName(fields.Meta, fields.Kind, fields.RawInput),
		Content:   normalizeToolContent(fields.Content),
		Locations: normalizeToolLocations(fields.Locations),
		RawInput:  boundedRawInput(fields.RawInput),
		RawOutput: boundedRawOutput(fields.RawOutput),
		Runtime:   acpToolRuntime(fields.Meta, fields.At),
	}
	if fields.Status != nil {
		src.Status = string(*fields.Status)
	}
	return src
}

func kindString(kind *acpschema.ToolKind) string {
	if kind == nil {
		return ""
	}
	return string(*kind)
}

func boundedRawInput(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || len(raw) > maxToolRawInputBytes {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return nil
	}
	return redactToolMap(out)
}

func boundedRawOutput(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(raw) > maxToolRawOutputBytes {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	data, err := json.Marshal(redactToolValue(value))
	if err != nil || len(data) == 0 || len(data) > maxToolRawOutputBytes {
		return nil
	}
	return json.RawMessage(data)
}

func normalizeToolLocations(locations []acpschema.ToolCallLocation) []sessionevents.ACPToolLocation {
	out := make([]sessionevents.ACPToolLocation, 0, len(locations))
	for _, location := range locations {
		if location.Path == "" {
			continue
		}
		out = append(out, sessionevents.ACPToolLocation{
			Path: redactToolText(location.Path),
			Line: location.Line,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizedToolName(meta map[string]any, kind *acpschema.ToolKind, rawInput json.RawMessage) string {
	if cc, ok := meta["claudeCode"].(map[string]any); ok {
		if name, ok := cc["toolName"].(string); ok && name != "" {
			return name
		}
	}
	if kind == nil || *kind != acpschema.ToolKindFetch {
		return ""
	}
	var input struct {
		Action struct {
			Type string `json:"type"`
		} `json:"action"`
	}
	if json.Unmarshal(rawInput, &input) == nil && input.Action.Type == "search" {
		return "WebSearch"
	}
	return "WebFetch"
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
			OldText string `json:"oldText"`
			NewText string `json:"newText"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil
		}
		return &sessionevents.ACPToolContent{
			Type:    "diff",
			Path:    redactToolText(d.Path),
			OldText: clampToolText(d.OldText),
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
			URI:   redactToolText(block.URI),
			Title: redactToolText(firstNonEmpty(block.Title, block.Name)),
		}
	case "resource":
		var res struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		}
		if len(block.Resource) > 0 {
			_ = json.Unmarshal(block.Resource, &res)
		}
		uri := redactToolText(res.URI)
		if text := clampToolText(res.Text); text != "" {
			return &sessionevents.ACPToolContent{Type: "text", Text: text, URI: uri}
		}
		if uri != "" {
			return &sessionevents.ACPToolContent{Type: "link", URI: uri}
		}
		return nil
	default:
		// image / audio / unknown blocks carry no inline text worth showing.
		return nil
	}
}

func clampToolText(s string) string {
	s = redactToolText(s)
	if utf8.RuneCountInString(s) <= maxToolContentText {
		return s
	}
	return string([]rune(s)[:maxToolContentText]) + "…"
}

func redactToolText(s string) string {
	s = oauthJSONFieldPattern.ReplaceAllString(s, `${1}[REDACTED]${2}`)
	s = bearerTokenPattern.ReplaceAllString(s, `${1}[REDACTED]`)
	s = googleAccessPattern.ReplaceAllString(s, `[REDACTED]`)
	s = googleRefreshPattern.ReplaceAllString(s, `[REDACTED]`)
	return s
}

func redactToolMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[redactToolText(key)] = redactToolValue(value)
	}
	return out
}

func redactToolValue(value any) any {
	switch v := value.(type) {
	case string:
		return redactToolText(v)
	case map[string]any:
		return redactToolMap(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, redactToolValue(item))
		}
		return out
	default:
		return value
	}
}
