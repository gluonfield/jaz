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
		{"viewport units", `<div style="height: 100vh">x</div>`, "viewport units"},
		{"screen class", `<div class="min-h-screen">x</div>`, "viewport units"},
		{"hex color", `<div style="color: #ff0000">x</div>`, "hardcoded color"},
		{"rgb color", `<div style="background: rgb(255, 0, 0)">x</div>`, "hardcoded color"},
		{"stock palette", `<span class="bg-red-500">x</span>`, "compiles to NOTHING"},
		{"fixed position", `<div style="position: fixed">x</div>`, "position: fixed"},
		{"important abuse", `<style>a{color:red !important;b{x:1 !important}c{y:2 !important}</style>`, "!important"},
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
	clean := `<div class="jz-stack"><div class="jz-kpis"><div class="jz-stat"><div class="jz-stat-value">42</div></div></div><ul class="jz-list jz-fill jz-scroll"><li class="jz-item"><span class="jz-item-title">A</span><span class="jz-item-value tabular-nums text-primary">7</span></li></ul></div>`
	if warnings := widgets.LintHTML(clean); len(warnings) != 0 {
		t.Fatalf("clean fragment produced warnings: %v", warnings)
	}
}
