package coordinator

import (
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/templates/jazplatform"
)

// memoryData loads the jazmem horizons injected into every turn. The memory
// protocol text lives in jazplatform.tmpl next to the rendered block. The
// horizons always render — "(empty)" when blank — so every agent sees the
// memory structure; daily pages appear only once something was captured. Memory
// files are injected whole so agents can see the complete horizon state.
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
		Root:      strings.TrimSpace(memoryRoot),
		LongTerm:  orEmpty(longTerm),
		ShortTerm: orEmpty(shortTerm),
	}
	data.TodayName = fmt.Sprintf("daily/%s.md", now.Local().Format("2006-01-02"))
	today, err := ReadPromptFile(memoryRoot, data.TodayName)
	if err != nil {
		return nil, err
	}
	data.Today = today
	return data, nil
}

func orEmpty(content string) string {
	if content == "" {
		return "(empty)"
	}
	return content
}
