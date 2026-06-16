package visualizeguide

import (
	"strings"
	"testing"
)

func TestForMapsModulesToSections(t *testing.T) {
	cases := []struct {
		modules []string
		want    Data
	}{
		{[]string{"chart"}, Data{UIComponents: true, ColorPalette: true, Charts: true, GeoMaps: true}},
		{[]string{"data_viz"}, Data{UIComponents: true, ColorPalette: true, Charts: true, GeoMaps: true}},
		{[]string{"diagram"}, Data{ColorPalette: true, SVGSetup: true, DiagramTypes: true}},
		{[]string{"mockup"}, Data{UIComponents: true, ColorPalette: true}},
		{[]string{"interactive"}, Data{UIComponents: true, ColorPalette: true}},
		// art and elicitation do NOT contribute the color palette — parity with Claude.
		{[]string{"art"}, Data{SVGSetup: true, Art: true}},
		{[]string{"elicitation"}, Data{Elicitation: true}},
		{[]string{"CHART", " Diagram "}, Data{UIComponents: true, ColorPalette: true, Charts: true, GeoMaps: true, SVGSetup: true, DiagramTypes: true}},
		{[]string{"nonsense"}, Data{}},
		{nil, Data{}},
	}
	for _, tc := range cases {
		if got := For(tc.modules); got != tc.want {
			t.Fatalf("For(%v) = %+v, want %+v", tc.modules, got, tc.want)
		}
	}
}

// TestParityWithClaude locks in the exact per-module section sets and ordering
// observed in Claude's visualize:read_me tool output.
func TestParityWithClaude(t *testing.T) {
	// art and elicitation must exclude the color palette.
	for _, module := range []string{"art", "elicitation"} {
		if strings.Contains(Render(For([]string{module})), "## Color palette") {
			t.Fatalf("%s guide must not include the color palette (parity with Claude)", module)
		}
	}
	// diagram includes the color palette but not UI components.
	diagram := Render(For([]string{"diagram"}))
	if !strings.Contains(diagram, "## Color palette") || strings.Contains(diagram, "## UI components") {
		t.Fatal("diagram guide must include Color palette and exclude UI components")
	}
	// In chart/mockup output, UI components precedes Color palette.
	chart := Render(For([]string{"chart"}))
	if i, j := strings.Index(chart, "## UI components"), strings.Index(chart, "## Color palette"); i < 0 || j < 0 || i > j {
		t.Fatalf("UI components (%d) must precede Color palette (%d) in chart output", i, j)
	}
}

func TestRenderFullContainsEverySection(t *testing.T) {
	full := Render(Full())
	for _, want := range []string{
		"# Imagine — Visual Creation Suite",
		"## Modules",
		"## Core Design System",
		"## When nothing fits",
		"## Color palette",
		"## SVG setup",
		"## Diagram types",
		"## UI components",
		"## Charts (Chart.js)",
		"## Geographic maps (D3 choropleth)",
		"## Art and illustration",
		"## Elicitation — collecting skill arguments",
		"Selected files appear as 120×120 tiles",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("full guide missing %q", want)
		}
	}
}

func TestRenderChartFiltersAndStaysClean(t *testing.T) {
	guide := Render(For([]string{"chart"}))

	for _, want := range []string{"## Core Design System", "## Color palette", "## UI components", "## Charts (Chart.js)", "## Geographic maps (D3 choropleth)"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("chart guide missing %q", want)
		}
	}
	for _, skip := range []string{"## SVG setup", "## Diagram types", "## Art and illustration", "## Elicitation"} {
		if strings.Contains(guide, skip) {
			t.Fatalf("chart guide should exclude %q", skip)
		}
	}
	if len(guide) > 50_000 {
		t.Fatalf("chart guide = %d bytes, want under the tool-result budget", len(guide))
	}
	assertClean(t, guide)
}

func TestRenderEmptyIsCoreOnly(t *testing.T) {
	core := Render(For(nil))
	for _, want := range []string{"## Modules", "## Core Design System", "## When nothing fits"} {
		if !strings.Contains(core, want) {
			t.Fatalf("core guide must include always-on section %q", want)
		}
	}
	// Every module-contributed section — including the color palette — is gated off.
	for _, skip := range []string{"## UI components", "## Color palette", "## Charts (Chart.js)", "## Diagram types", "## Elicitation"} {
		if strings.Contains(core, skip) {
			t.Fatalf("core guide should exclude module section %q", skip)
		}
	}
	assertClean(t, core)
}

// assertClean guards the whitespace contract: no leaked template directives, no
// blank-line gaps left by gated-out sections, exactly one trailing newline.
func assertClean(t *testing.T, guide string) {
	t.Helper()
	if strings.Contains(guide, "{{") || strings.Contains(guide, "}}") {
		t.Fatal("guide leaked a template directive")
	}
	if strings.Contains(guide, "\n\n\n") {
		t.Fatal("guide has a triple newline (gated section left a gap)")
	}
	if !strings.HasSuffix(guide, "\n") || strings.HasSuffix(guide, "\n\n") {
		t.Fatal("guide must end with exactly one trailing newline")
	}
}
