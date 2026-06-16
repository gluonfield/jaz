package widgets

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/templates/widgetprompt"
)

func PromptSection(loop loops.Loop, widget *Widget) string {
	data := widgetprompt.Data{
		FilePath: WidgetFilePath(loop),
	}
	if data.FilePath != "" {
		if info, err := os.Stat(data.FilePath); err == nil && info.Mode().IsRegular() {
			data.FileExists = true
		}
	}
	if widget != nil {
		data.Published = true
		data.Version = widget.CurrentVersion
		data.Title = widget.Title
		data.SizeHint = widget.SizeHint
		data.LastError = widget.LastError
		data.LayoutFeedback = layoutFeedback(widget.LastLayout)
	}
	return widgetprompt.Render(data)
}

// layoutReport mirrors the bridge's jaz:artifact-layout measurement payload.
type layoutReport struct {
	DeadSpacePct int `json:"dead_space_pct"`
	OverflowPx   int `json:"overflow_px"`
	Clipped      int `json:"clipped"`
	ImgErrors    int `json:"img_errors"`
}

// layoutFeedback turns stored telemetry into actionable prompt text; empty
// when the layout is healthy so healthy widgets cost no tokens.
func layoutFeedback(payload string) string {
	if strings.TrimSpace(payload) == "" {
		return ""
	}
	var r layoutReport
	if json.Unmarshal([]byte(payload), &r) != nil {
		return ""
	}
	var parts []string
	if r.DeadSpacePct >= 20 {
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
