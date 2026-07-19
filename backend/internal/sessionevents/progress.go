package sessionevents

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxProgressEntryContentRunes = 240

func NormalizePlanEntries(entries []PlanEntry) ([]PlanEntry, bool) {
	out := make([]PlanEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Content = strings.TrimSpace(entry.Content)
		if entry.Content == "" {
			return nil, false
		}
		out = append(out, entry)
	}
	return out, true
}

func NormalizeProgressEntries(entries []PlanEntry) ([]PlanEntry, bool) {
	out, ok := NormalizePlanEntries(entries)
	if !ok {
		return nil, false
	}
	for _, entry := range out {
		if utf8.RuneCountInString(entry.Content) > maxProgressEntryContentRunes ||
			strings.ContainsAny(entry.Content, "\r\n") || looksLikeMarkdownBlock(entry.Content) {
			return nil, false
		}
	}
	return out, true
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
