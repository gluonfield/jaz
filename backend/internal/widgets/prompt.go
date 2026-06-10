package widgets

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/loops"
)

// What the loop's agent is told: a short prompt section with the contract
// essentials, backed by the full style guide kept on disk next to the widget.

//go:embed assets/widget-guide.md
var GuideMD string

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
	path := WidgetFilePath(loop)
	var b strings.Builder
	b.WriteString("## Widget instructions\n\n")
	b.WriteString("- This loop owns a widget: a small HTML tile on the user's Jaz board, updated by you on every run.\n")
	if path != "" {
		fmt.Fprintf(&b, "- Widget file: %s (iterate on the existing file rather than starting over).\n", path)
	}
	if guide := WidgetGuidePath(loop); guide != "" {
		fmt.Fprintf(&b, "- Before editing, read the style guide next to it: %s. It defines the required design system (Jaz palette + Tailwind), the fill-the-tile layout rules, and the quality bar. Follow it.\n", guide)
	}
	b.WriteString("- The file must be a self-contained HTML FRAGMENT (no <!doctype>, <html>, <head>, or <body>), under 1 MB. It lives outside the session workspace; if a file tool refuses the path, write it via the shell (like the memory file).\n")
	b.WriteString("- Publish by calling the publish_widget tool or the _jaz.dev/widget/publish extension method if available (it validates and returns errors); otherwise the file is published automatically when the run finishes.\n")
	b.WriteString("- Show ONLY the data the user asked the loop to track — the tile chrome already shows the title and freshness; no meta commentary, captions, or repeated headings. Read-only: no forms or action buttons.\n")
	if widget != nil {
		fmt.Fprintf(&b, "Current widget: version %d, title %q", widget.CurrentVersion, widget.Title)
		if widget.SizeHint != "" {
			fmt.Fprintf(&b, ", size %s", widget.SizeHint)
		}
		b.WriteString(".\n")
		if widget.LastError != "" {
			fmt.Fprintf(&b, "Runtime error reported by the board since the last publish (fix it this run): %s\n", widget.LastError)
		}
		if feedback := layoutFeedback(widget.LastLayout); feedback != "" {
			fmt.Fprintf(&b, "Board layout telemetry for the current version (fix this run): %s.\n", feedback)
		}
	} else {
		b.WriteString("Current widget: never published. Create the first version this run.\n")
	}
	return b.String()
}

// layoutReport mirrors the bridge's jaz:layout measurement payload.
type layoutReport struct {
	DeadSpacePct int `json:"dead_space_pct"`
	OverflowPx   int `json:"overflow_px"`
	Clipped      int `json:"clipped"`
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
		parts = append(parts, fmt.Sprintf("~%d%% of the tile is empty at the bottom — let the main content area grow (jz-fill) or publish a smaller size_hint", r.DeadSpacePct))
	}
	if r.OverflowPx > 8 {
		parts = append(parts, fmt.Sprintf("content overflows the tile by ~%dpx — designate one internal scroller (jz-fill jz-scroll) or tighten the layout", r.OverflowPx))
	}
	if r.Clipped > 0 {
		parts = append(parts, fmt.Sprintf("%d element(s) clip their content with overflow:hidden — never crop content to fit; let it scroll or size it by the available space", r.Clipped))
	}
	return strings.Join(parts, "; ")
}
