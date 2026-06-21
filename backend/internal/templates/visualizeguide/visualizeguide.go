// Package visualizeguide renders the inline-artifact design guidance, scoped to
// the modules a request needs. The full guide is ~90 KB; returning all of it
// overflows the agent's tool-result cap, so callers pass only the modules in
// play (e.g. "chart") and get the design-system core plus those sections.
package visualizeguide

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

//go:embed visualizeguide.tmpl
var guideTemplate string

var tmpl = template.Must(template.New("visualizeguide").Parse(guideTemplate))

// Data toggles the optional sections of the guide. The module index, core design
// system, and "when nothing fits" always render; every field here is an optional
// section gated in the template, in the canonical order the renderer expects.
type Data struct {
	UIComponents bool
	ColorPalette bool
	SVGSetup     bool
	DiagramTypes bool
	Charts       bool
	GeoMaps      bool
	Art          bool
	Elicitation  bool
	Mobile       bool
}

// For maps requested modules to the sections they need — the single place that
// derives guide structure from requirements. The color palette is contributed by the visual modules (diagram,
// mockup, interactive, chart) but not by art or elicitation. Unknown modules
// contribute nothing, so an empty or unrecognized list yields just the core.
func For(modules []string, platform string) Data {
	var d Data
	for _, module := range modules {
		switch strings.ToLower(strings.TrimSpace(module)) {
		case "diagram":
			d.ColorPalette, d.SVGSetup, d.DiagramTypes = true, true, true
		case "mockup", "interactive":
			d.UIComponents, d.ColorPalette = true, true
		case "chart", "data_viz":
			d.UIComponents, d.ColorPalette, d.Charts, d.GeoMaps = true, true, true, true
		case "art":
			d.SVGSetup, d.Art = true, true
		case "elicitation":
			d.Elicitation = true
		}
	}
	if platform == "mobile" {
		d.Mobile = true
	}
	return d
}

// Render assembles the guide for the requested sections, normalized to a single
// trailing newline. The template is embedded and parse-checked at init by
// template.Must, so a panic here means a programmer edit broke the template.
func Render(data Data) string {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic(err)
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}
