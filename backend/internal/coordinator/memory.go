package coordinator

import (
	"fmt"
	"strings"
	"time"

	prompttemplate "github.com/wins/jaz/backend/internal/templates/coordinator"
)

// Memory horizon budgets, in characters. LONG_TERM and SHORT_TERM are curated
// (most important first) so they truncate from the head; daily pages are
// chronological logs so they keep the tail (latest entries).
const (
	longTermPromptChars  = 2500
	shortTermPromptChars = 1500
	dailyPromptChars     = 1200
)

// memorySections loads the always-injected jazmem horizons: LONG_TERM.md,
// SHORT_TERM.md, and today's + yesterday's daily pages. Missing files are
// skipped; the memory root is jazmem's markdown root, separate from the jaz
// root that holds AGENTS.md.
func memorySections(memoryRoot string, now time.Time) []prompttemplate.Section {
	if strings.TrimSpace(memoryRoot) == "" {
		return nil
	}
	var sections []prompttemplate.Section
	add := func(name, content string) {
		if content != "" {
			sections = append(sections, prompttemplate.Section{Name: "memory/" + name, Body: content})
		}
	}
	longTerm, _ := ReadPromptFile(memoryRoot, "LONG_TERM.md")
	add("LONG_TERM.md", truncateHead(longTerm, longTermPromptChars))
	shortTerm, _ := ReadPromptFile(memoryRoot, "SHORT_TERM.md")
	add("SHORT_TERM.md", truncateHead(shortTerm, shortTermPromptChars))
	for _, day := range []time.Time{now, now.AddDate(0, 0, -1)} {
		name := fmt.Sprintf("daily/%s.md", day.Local().Format("2006-01-02"))
		daily, _ := ReadPromptFile(memoryRoot, name)
		add(name, truncateTail(daily, dailyPromptChars))
	}
	return sections
}

func truncateHead(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n...[truncated]"
}

func truncateTail(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return "...[truncated]\n" + content[len(content)-maxChars:]
}
