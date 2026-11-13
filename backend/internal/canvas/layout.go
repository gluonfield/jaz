package canvas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const LayoutFileName = "layout.json"

// Box is an item as a client drew it: its place and its measured size.
type Box struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Layout is what a client saw of one page, for the agent to read back.
type Layout struct {
	Items  []Box    `json:"items"`
	Issues []string `json:"issues"`
}

// SaveLayout keeps the latest layout of each page beside the canvas, one item
// and one issue per line, and forgets pages that are gone.
func (s *Service) SaveLayout(sessionID, page string, layout Layout) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.Dir(sessionID)
	_, document, err := readDocument(dir)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(document.Pages, func(known Page) bool { return known.ID == page }) {
		return fmt.Errorf("%w: page %q does not exist", ErrInvalid, page)
	}
	var stored struct {
		Pages map[string]Layout `json:"pages"`
	}
	// A layout that cannot be read is only a stale measure: it is replaced.
	if raw, err := os.ReadFile(filepath.Join(dir, LayoutFileName)); err == nil {
		_ = json.Unmarshal(raw, &stored)
	}
	if stored.Pages == nil {
		stored.Pages = map[string]Layout{}
	}
	stored.Pages[page] = layout
	var out bytes.Buffer
	out.WriteString("{\n  \"pages\": {")
	written := 0
	for _, known := range document.Pages {
		layout, ok := stored.Pages[known.ID]
		if !ok {
			continue
		}
		if written > 0 {
			out.WriteString(",")
		}
		written++
		id, _ := json.Marshal(known.ID)
		fmt.Fprintf(&out, "\n    %s: {\"items\":[", id)
		if err := writeLines(&out, layout.Items); err != nil {
			return err
		}
		out.WriteString("],\"issues\":[")
		if err := writeLines(&out, layout.Issues); err != nil {
			return err
		}
		out.WriteString("]}")
	}
	out.WriteString("\n  }\n}\n")
	return writeFile(dir, LayoutFileName, out.Bytes())
}
