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

// memoryInstructions is the always-present memory protocol; the jazmem skill
// holds the full conventions, but capture behavior cannot depend on the agent
// choosing to load a skill.
const memoryInstructions = `Jaz has persistent markdown memory (jazmem) at ~/.jaz/memory. The memory/* sections below are live files, re-read every turn: LONG_TERM.md (who the user is and where they are headed), SHORT_TERM.md (current focus, active projects, open loops), and recent daily pages (raw log).

- Search memory before answering about people, projects, preferences, decisions, or prior work: mem_search (cited answer) or the jazmem CLI for raw retrieval; escalate per the jazmem skill before concluding memory is missing.
- Capture as you go, not at session end: when you learn something durable, append it to today's daily page (memory/daily/YYYY-MM-DD.md) and run jazmem index.
- Keep SHORT_TERM.md true about the present: when focus or open loops change, update the affected lines in place.
- Never edit LONG_TERM.md; dream maintains it nightly from cited daily/inbox material.
- Cite durable facts ([Source: ..., YYYY-MM-DD], absolute dates); uncertain material goes to memory/inbox/ with exact wording.
- Full conventions: the jazmem skill.`

// memorySections injects the memory protocol plus the jazmem horizons:
// LONG_TERM.md, SHORT_TERM.md, and today's + yesterday's daily pages. Missing
// files are skipped (ReadPromptFile returns ""); real read errors propagate
// like the jaz prompt files so memory never vanishes silently.
func memorySections(memoryRoot string, now time.Time) ([]prompttemplate.Section, error) {
	if strings.TrimSpace(memoryRoot) == "" {
		return nil, nil
	}
	sections := []prompttemplate.Section{{Name: "memory", Body: memoryInstructions}}
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
