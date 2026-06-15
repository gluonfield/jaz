package widgets_test

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/widgets"
)

func TestLintHTMLFlagsAntipatterns(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"full document", `<!doctype html><html><body><p>x</p></body></html>`, "fragment, not a full document"},
		{"head element", `<head><title>x</title></head><div>x</div>`, "fragment, not a full document"},
		{"viewport height", `<div style="height: 100vh">x</div>`, "viewport units"},
		{"viewport width", `<div style="width: 50vw">x</div>`, "viewport units"},
		{"fixed position", `<div style="position: fixed">x</div>`, "position: fixed"},
	}
	for _, tc := range cases {
		warnings := widgets.LintHTML(tc.html)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, tc.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected warning containing %q, got %v", tc.name, tc.want, warnings)
		}
	}
}

func TestLintHTMLCleanFragment(t *testing.T) {
	// An artifact-style fragment: design-system vars, a ramp class, a fill-the-
	// tile flex column, and a hardcoded series color (legitimate on canvas).
	clean := `<div style="height:100%;display:flex;flex-direction:column;color:var(--color-text-primary)"><div class="c-blue" style="padding:8px">42</div><div style="flex:1;overflow:auto"><canvas></canvas></div></div>`
	if warnings := widgets.LintHTML(clean); len(warnings) != 0 {
		t.Fatalf("clean fragment produced warnings: %v", warnings)
	}
}
