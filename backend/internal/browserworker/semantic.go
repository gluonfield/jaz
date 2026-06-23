package browserworker

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	stateTextLimit    = 1200
	stateElementLimit = 40
	extractItemLimit  = 24
	extractNavLimit   = 12
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

type PageExtraction struct {
	URL        string          `json:"url"`
	Title      string          `json:"title"`
	ReadyState string          `json:"ready_state"`
	Coverage   ExtractCoverage `json:"coverage"`
	Items      []ExtractedItem `json:"items"`
	Navigation []StateElement  `json:"navigation"`
}

type ExtractCoverage struct {
	ScrollY        int  `json:"scroll_y"`
	ViewportHeight int  `json:"viewport_height"`
	DocumentHeight int  `json:"document_height"`
	ScrollPercent  int  `json:"scroll_percent"`
	AtBottom       bool `json:"at_bottom"`
}

type ExtractedItem struct {
	Ref   string `json:"ref"`
	Role  string `json:"role,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
	Href  string `json:"href,omitempty"`
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

func formatPageExtraction(extraction PageExtraction) string {
	var b strings.Builder
	if extraction.URL != "" {
		b.WriteString("URL: ")
		b.WriteString(shortenText(extraction.URL, 300))
		b.WriteByte('\n')
	}
	if extraction.Title != "" {
		b.WriteString("Title: ")
		b.WriteString(shortenText(extraction.Title, 180))
		b.WriteByte('\n')
	}
	b.WriteString(fmt.Sprintf("Coverage: visible_items=%d scroll=%d%% at_bottom=%t document=%dpx viewport=%dpx\n",
		len(extraction.Items),
		extraction.Coverage.ScrollPercent,
		extraction.Coverage.AtBottom,
		extraction.Coverage.DocumentHeight,
		extraction.Coverage.ViewportHeight,
	))
	if len(extraction.Items) > 0 {
		b.WriteString("\nItems:\n")
		limit := len(extraction.Items)
		if limit > extractItemLimit {
			limit = extractItemLimit
		}
		for i, item := range extraction.Items[:limit] {
			line := formatExtractedItem(i+1, item)
			if line == "" {
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if len(extraction.Items) > limit {
			b.WriteString("... items truncated ...\n")
		}
	}
	if len(extraction.Navigation) > 0 {
		b.WriteString("\nNavigation targets:\n")
		limit := len(extraction.Navigation)
		if limit > extractNavLimit {
			limit = extractNavLimit
		}
		for _, element := range extraction.Navigation[:limit] {
			line := formatStateElement(element)
			if line == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if len(extraction.Items) == 0 && len(extraction.Navigation) == 0 {
		b.WriteString("\nNo structured items or navigation targets were found in the visible page.")
	}
	return strings.TrimSpace(b.String())
}

func formatExtractedItem(index int, item ExtractedItem) string {
	ref := strings.TrimSpace(item.Ref)
	if ref == "" {
		return ""
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("%d. ref=%s", index, ref))
	if role := strings.TrimSpace(item.Role); role != "" {
		parts = append(parts, "role="+role)
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		parts = append(parts, fmt.Sprintf("%q", shortenText(title, 180)))
	}
	if href := strings.TrimSpace(item.Href); href != "" {
		parts = append(parts, shortenText(href, 180))
	}
	line := strings.Join(parts, " ")
	if text := strings.TrimSpace(item.Text); text != "" && !sameFold(text, item.Title) {
		line += "\n   " + shortenText(text, 300)
	}
	return line
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

func decodePageExtraction(data json.RawMessage) (PageExtraction, bool) {
	if len(data) == 0 {
		return PageExtraction{}, false
	}
	var extraction PageExtraction
	if json.Unmarshal(data, &extraction) != nil {
		return PageExtraction{}, false
	}
	return extraction, true
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
