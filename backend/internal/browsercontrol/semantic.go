package browsercontrol

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	stateTextLimit        = 1200
	stateElementLimit     = 40
	stateJSONLimit        = 1 << 20
	stateDataTextLimit    = 5000
	stateDataElementLimit = 80
	stateRefLimit         = 512
	stateRevisionLimit    = 200
	stateNameLimit        = 500
	stateElementTextLimit = 1000
	stateHrefLimit        = 2000
	stateValueLimit       = 5000
	stateValidationLimit  = 500
)

type PageState struct {
	URL          string         `json:"url"`
	Title        string         `json:"title"`
	ReadyState   string         `json:"ready_state"`
	PageRevision string         `json:"page_revision"`
	Text         string         `json:"text"`
	Elements     []StateElement `json:"elements"`
}

type StateElement struct {
	Ref        string `json:"ref"`
	Tag        string `json:"tag"`
	Role       string `json:"role,omitempty"`
	Name       string `json:"name,omitempty"`
	Text       string `json:"text,omitempty"`
	Href       string `json:"href,omitempty"`
	InputType  string `json:"input_type,omitempty"`
	Value      string `json:"value,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Disabled   bool   `json:"disabled,omitempty"`
	ReadOnly   bool   `json:"read_only,omitempty"`
	Checked    bool   `json:"checked,omitempty"`
	Validation string `json:"validation,omitempty"`
}

func formatPageState(state PageState) string {
	var b strings.Builder
	if state.URL != "" {
		b.WriteString("URL: ")
		b.WriteString(shortenText(state.URL, 300))
		b.WriteByte('\n')
	}
	if state.Title != "" {
		b.WriteString("Title: ")
		b.WriteString(shortenText(state.Title, 180))
		b.WriteByte('\n')
	}
	if state.ReadyState != "" {
		b.WriteString("Ready state: ")
		b.WriteString(state.ReadyState)
		b.WriteByte('\n')
	}
	if state.PageRevision != "" {
		b.WriteString("Page revision: ")
		b.WriteString(state.PageRevision)
		b.WriteByte('\n')
	}
	if len(state.Elements) > 0 {
		b.WriteString("\nTargets:\n")
		limit := len(state.Elements)
		if limit > stateElementLimit {
			limit = stateElementLimit
		}
		for _, element := range state.Elements[:limit] {
			line := formatStateElement(element)
			if line == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if len(state.Elements) > limit {
			b.WriteString("... targets truncated ...\n")
		}
	}
	text := shortenText(state.Text, stateTextLimit)
	if text != "" {
		b.WriteString("\nVisible text:\n")
		b.WriteString(text)
		if len(state.Text) > len(text) {
			b.WriteString("\n[truncated]")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatStateElement(element StateElement) string {
	ref := strings.TrimSpace(element.Ref)
	if ref == "" {
		return ""
	}
	var parts []string
	parts = append(parts, "ref="+ref)
	if tag := strings.TrimSpace(element.Tag); tag != "" {
		parts = append(parts, tag)
	}
	if role := strings.TrimSpace(element.Role); role != "" {
		parts = append(parts, "role="+role)
	}
	if name := strings.TrimSpace(element.Name); name != "" {
		parts = append(parts, fmt.Sprintf("%q", shortenText(name, 120)))
	}
	if text := strings.TrimSpace(element.Text); text != "" && !sameFold(text, element.Name) {
		parts = append(parts, "text="+fmt.Sprintf("%q", shortenText(text, 120)))
	}
	if href := strings.TrimSpace(element.Href); href != "" {
		parts = append(parts, shortenText(href, 160))
	}
	if inputType := strings.TrimSpace(element.InputType); inputType != "" {
		parts = append(parts, "type="+inputType)
	}
	if value := strings.TrimSpace(element.Value); value != "" {
		parts = append(parts, "value="+fmt.Sprintf("%q", shortenText(value, 120)))
	}
	if element.Required {
		parts = append(parts, "required")
	}
	if element.Disabled {
		parts = append(parts, "disabled")
	}
	if element.ReadOnly {
		parts = append(parts, "read-only")
	}
	if element.Checked {
		parts = append(parts, "checked")
	}
	if validation := strings.TrimSpace(element.Validation); validation != "" {
		parts = append(parts, "validation="+fmt.Sprintf("%q", shortenText(validation, 160)))
	}
	return strings.Join(parts, " ")
}

func decodePageState(data json.RawMessage) (PageState, bool) {
	if len(data) == 0 || len(data) > stateJSONLimit {
		return PageState{}, false
	}
	var state PageState
	if json.Unmarshal(data, &state) != nil {
		return PageState{}, false
	}
	state.URL = shortenText(state.URL, 1000)
	state.Title = shortenText(state.Title, 500)
	state.ReadyState = shortenText(state.ReadyState, 50)
	state.PageRevision = shortenText(state.PageRevision, stateRevisionLimit)
	state.Text = shortenText(state.Text, stateDataTextLimit)
	if len(state.Elements) > stateDataElementLimit {
		state.Elements = state.Elements[:stateDataElementLimit]
	}
	elements := state.Elements[:0]
	for _, element := range state.Elements {
		ref := strings.TrimSpace(element.Ref)
		if ref == "" || len(ref) > stateRefLimit {
			continue
		}
		element.Ref = ref
		element.Tag = shortenText(element.Tag, 50)
		element.Role = shortenText(element.Role, 100)
		element.Name = shortenText(element.Name, stateNameLimit)
		element.Text = shortenText(element.Text, stateElementTextLimit)
		element.Href = truncateUTF8(strings.TrimSpace(element.Href), stateHrefLimit)
		element.InputType = shortenText(element.InputType, 100)
		element.Value = truncateUTF8(element.Value, stateValueLimit)
		element.Validation = shortenText(element.Validation, stateValidationLimit)
		if strings.EqualFold(element.InputType, "password") {
			element.Text = ""
			element.Value = ""
		}
		elements = append(elements, element)
	}
	state.Elements = elements
	return state, true
}

func shortenText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	return truncateUTF8(value, limit)
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

func sameFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
