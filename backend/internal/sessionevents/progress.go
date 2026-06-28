package sessionevents

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxProgressEntryContentRunes = 240

func NormalizeProgressEntries(entries []PlanEntry) ([]PlanEntry, bool) {
	out := make([]PlanEntry, 0, len(entries))
	for _, entry := range entries {
		content, ok := NormalizeProgressEntryContent(entry.Content)
		if !ok {
			return nil, false
		}
		entry.Content = content
		out = append(out, entry)
	}
	return out, true
}

func NormalizeProgressEntryContent(content string) (string, bool) {
	text := strings.TrimSpace(content)
	if text == "" || utf8.RuneCountInString(text) > maxProgressEntryContentRunes || strings.ContainsAny(text, "\r\n") {
		return "", false
	}
	if looksLikeMarkdownBlock(text) {
		return "", false
	}
	return text, true
}

func NormalizePlanDocumentText(entries []PlanEntry) (string, bool) {
	if len(entries) != 1 {
		return "", false
	}
	text := strings.TrimSpace(entries[0].Content)
	if text == "" || text == "#" {
		return "", false
	}
	if _, ok := NormalizeProgressEntryContent(text); ok {
		return "", false
	}
	return text, true
}

func looksLikeMarkdownBlock(text string) bool {
	switch {
	case text == "#":
		return true
	case strings.HasPrefix(text, "# "):
		return true
	case strings.HasPrefix(text, "##"):
		return true
	case strings.HasPrefix(text, "- "):
		return true
	case strings.HasPrefix(text, "* "):
		return true
	case strings.HasPrefix(text, "+ "):
		return true
	case strings.HasPrefix(text, "> "):
		return true
	case strings.HasPrefix(text, "```"):
		return true
	case strings.HasPrefix(text, "|"):
		return true
	default:
		return startsWithOrderedListMarker(text)
	}
}

func startsWithOrderedListMarker(text string) bool {
	for i, r := range text {
		if unicode.IsDigit(r) {
			continue
		}
		if (r == '.' || r == ')') && i > 0 {
			rest := text[i+1:]
			return strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t")
		}
		return false
	}
	return false
}
