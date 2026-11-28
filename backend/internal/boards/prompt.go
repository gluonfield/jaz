package boards

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/templates/boardprompt"
)

// PromptSection describes the board app a loop run must publish.
func PromptSection(loop loops.Loop, loopApp storage.LoopApp, app storage.App) string {
	return boardprompt.Render(boardprompt.Data{
		FilePath:       AppFilePath(loop),
		Published:      app.CurrentVersion > 0,
		Version:        app.CurrentVersion,
		Title:          app.Title,
		SizeHint:       loopApp.SizeHint,
		LastError:      loopApp.LastError,
		LayoutFeedback: layoutFeedback(loopApp.LastLayout, app.CurrentVersion),
	})
}

// layoutReport mirrors the bridge's jaz:artifact-layout measurement payload.
type layoutReport struct {
	DeadSpacePct int `json:"dead_space_pct"`
	OverflowPx   int `json:"overflow_px"`
	Clipped      int `json:"clipped"`
	ImgErrors    int `json:"img_errors"`
}

// layoutFeedback turns stored telemetry into actionable prompt text; empty
// when the layout is healthy so healthy tiles cost no tokens.
func layoutFeedback(payload string, version int) string {
	if strings.TrimSpace(payload) == "" {
		return ""
	}
	var r layoutReport
	if json.Unmarshal([]byte(payload), &r) != nil {
		return ""
	}
	var parts []string
	if version > 1 && r.DeadSpacePct >= 20 {
		parts = append(parts, fmt.Sprintf("~%d%% of the tile is empty at the bottom — let the main content area grow to fill the height or publish a smaller size_hint", r.DeadSpacePct))
	}
	if r.OverflowPx > 8 {
		parts = append(parts, fmt.Sprintf("content overflows the tile by ~%dpx — give one region overflow:auto to scroll inside the tile, or tighten the layout", r.OverflowPx))
	}
	if r.Clipped > 0 {
		parts = append(parts, fmt.Sprintf("%d element(s) clip their content with overflow:hidden — never crop content to fit; let it scroll or size it by the available space", r.Clipped))
	}
	if r.ImgErrors > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) failed to load (the board hid them, leaving holes) — remove them or switch to URLs you verified this run", r.ImgErrors))
	}
	return strings.Join(parts, "; ")
}
