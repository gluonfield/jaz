package coordinator

import (
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/templates/jazplatform"
)

// Memory horizon budgets, in characters. LONG_TERM and SHORT_TERM are curated
// (most important first) so they truncate from the head; daily pages are
// chronological logs so they keep the tail (latest entries).
const (
	longTermPromptChars  = 2500
	shortTermPromptChars = 1500
	dailyPromptChars     = 1200
)

// memoryData loads the jazmem horizons injected into every turn. The memory
// protocol text lives in jazplatform.tmpl next to the rendered block. The
// horizons always render — "(empty)" when blank — so every agent sees the
// memory structure; daily pages appear only once something was captured.
// Missing files read as "" (ReadPromptFile); real read errors propagate so
// memory never vanishes silently.
func memoryData(memoryRoot string, now time.Time) (*jazplatform.MemoryData, error) {
	if strings.TrimSpace(memoryRoot) == "" {
		return nil, nil
	}
	longTerm, err := ReadPromptFile(memoryRoot, "LONG_TERM.md")
	if err != nil {
		return nil, err
	}
	shortTerm, err := ReadPromptFile(memoryRoot, "SHORT_TERM.md")
	if err != nil {
		return nil, err
	}
	data := &jazplatform.MemoryData{
		LongTerm:  orEmpty(truncateHead(longTerm, longTermPromptChars)),
		ShortTerm: orEmpty(truncateHead(shortTerm, shortTermPromptChars)),
	}
	data.TodayName = fmt.Sprintf("daily/%s.md", now.Local().Format("2006-01-02"))
	today, err := ReadPromptFile(memoryRoot, data.TodayName)
	if err != nil {
		return nil, err
	}
	data.Today = truncateTail(today, dailyPromptChars)
	return data, nil
}

func orEmpty(content string) string {
	if content == "" {
		return "(empty)"
	}
	return content
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
