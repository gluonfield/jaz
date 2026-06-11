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
// skipped (ReadPromptFile returns ""); real read errors propagate like the
// jaz prompt files so memory never vanishes silently.
func memorySections(memoryRoot string, now time.Time) ([]prompttemplate.Section, error) {
	if strings.TrimSpace(memoryRoot) == "" {
		return nil, nil
	}
	var sections []prompttemplate.Section
	add := func(name, content string) {
		if content != "" {
			sections = append(sections, prompttemplate.Section{Name: "memory/" + name, Body: content})
		}
	}
	longTerm, err := ReadPromptFile(memoryRoot, "LONG_TERM.md")
	if err != nil {
		return nil, err
	}
	add("LONG_TERM.md", truncateHead(longTerm, longTermPromptChars))
	shortTerm, err := ReadPromptFile(memoryRoot, "SHORT_TERM.md")
	if err != nil {
		return nil, err
	}
	add("SHORT_TERM.md", truncateHead(shortTerm, shortTermPromptChars))
	for _, day := range []time.Time{now, now.AddDate(0, 0, -1)} {
		name := fmt.Sprintf("daily/%s.md", day.Local().Format("2006-01-02"))
		daily, err := ReadPromptFile(memoryRoot, name)
		if err != nil {
			return nil, err
		}
		add(name, truncateTail(daily, dailyPromptChars))
	}
	return sections, nil
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
