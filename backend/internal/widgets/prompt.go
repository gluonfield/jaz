package widgets

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/templates/widgetprompt"
	"github.com/wins/jaz/backend/internal/visualize"
)

// What the loop's agent is told: a short prompt section with the contract
// essentials, backed by the full style guide kept on disk next to the widget.

//go:embed assets/tile-addendum.md
var tileAddendum string

// GuideMD is the design contract written next to the widget file. It is the
// inline-artifact design system (the same guide visualize_read_me serves) plus
// a short tile-mode addendum — so loops author widgets identically to artifacts.
var GuideMD = visualize.ReadMeGuide + tileAddendum

// GuideFileName sits next to the widget file; agents conventionally read
// AGENTS.md before touching a directory.
const GuideFileName = "AGENTS.md"

// WidgetGuidePath is where EnsureGuide writes the style guide for the loop.
func WidgetGuidePath(loop loops.Loop) string {
	dir := WidgetDir(loop)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, GuideFileName)
}

// EnsureGuide keeps the full design contract in a file next to the widget so
// the run prompt can stay short. Jaz owns the file: it is rewritten whenever
// the embedded guide changes.
func EnsureGuide(loop loops.Loop) (string, error) {
	path := WidgetGuidePath(loop)
	if path == "" {
		return "", nil
	}
	if data, err := os.ReadFile(path); err == nil && string(data) == GuideMD {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(GuideMD), 0o644)
}

// PromptSection is appended to a loop's metadata prompt when the loop is
// assigned to a board: the contract essentials plus current publish state.
// The design system and quality bar live in the guide file, not the prompt.
func PromptSection(loop loops.Loop, widget *Widget) string {
	data := widgetprompt.Data{
		FilePath:  WidgetFilePath(loop),
		GuidePath: WidgetGuidePath(loop),
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
	section, err := widgetprompt.Render(data)
	if err != nil {
		// Embedded and parse-checked at init; a run must not lose the contract.
		return "## Widget instructions\n\nUpdate the widget file for this loop and publish it.\n"
	}
	return section
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
