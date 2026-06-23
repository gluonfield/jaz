package browserworker

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	stateTextLimit    = 1800
	stateElementLimit = 80
)

type PageState struct {
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	ReadyState string         `json:"ready_state"`
	Text       string         `json:"text"`
	Elements   []StateElement `json:"elements"`
}

type StateElement struct {
	Ref      string `json:"ref"`
	Tag      string `json:"tag"`
	Role     string `json:"role,omitempty"`
	Name     string `json:"name,omitempty"`
	Text     string `json:"text,omitempty"`
	Href     string `json:"href,omitempty"`
	Selector string `json:"selector,omitempty"`
}

func formatPageState(state PageState) string {
	var b strings.Builder
	if state.URL != "" {
		b.WriteString("URL: ")
		b.WriteString(state.URL)
		b.WriteByte('\n')
	}
	if state.Title != "" {
		b.WriteString("Title: ")
		b.WriteString(state.Title)
		b.WriteByte('\n')
	}
	if state.ReadyState != "" {
		b.WriteString("Ready state: ")
		b.WriteString(state.ReadyState)
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
		parts = append(parts, href)
	}
	return strings.Join(parts, " ")
}

func decodePageState(data json.RawMessage) (PageState, bool) {
	if len(data) == 0 {
		return PageState{}, false
	}
	var state PageState
	if json.Unmarshal(data, &state) != nil {
		return PageState{}, false
	}
	return state, true
}

func shortenText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit])
}

func sameFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
