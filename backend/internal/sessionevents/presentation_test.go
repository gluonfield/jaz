package sessionevents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoredPresentationsNormalizeAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stored    string
		kind      PresentationKind
		file      string
		reference string
	}{
		{"inline", `{"presented":true,"title":"Map","widget_code":"<svg></svg>","artifact_type":"svg","data":{"x":1}}`, PresentationArtifact, "", ""},
		{"plugin", `{"presented":true,"title":"Status","widget_code":"<p></p>","artifact_type":"html","file":"status.json"}`, PresentationPluginView, "status.json", ""},
		{"referenced plugin", `{"kind":"plugin-view","presented":true,"view":{"reference":"tenders/status","title":"Status","widget_code":"<p></p>","file":"status.json"}}`, PresentationPluginView, "status.json", "tenders/status"},
		{"canvas", `{"presented":true,"title":"Canvas","widget_code":"","artifact_type":"canvas","file":"/private/runtime/canvases/id/canvas.json"}`, PresentationCanvas, "", ""},
		{"map", `{"kind":"map","presented":true,"map":{"title":"Coffee","places":[{"name":"Caravan","latitude":51.5357,"longitude":-0.125,"rating":4.4,"reviews":3100}]}}`, PresentationMap, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := Event{Type: TypeArtifact, Content: tc.stored}
			event.NormalizePayload()
			if event.Artifact == nil || event.Artifact.Kind != tc.kind || event.Content != "" || !event.Delivers() {
				t.Fatalf("normalized event = %+v", event)
			}
			wire, err := json.Marshal(event)
			if err != nil || strings.Contains(string(wire), "/private/runtime") {
				t.Fatalf("public presentation = %s, %v", wire, err)
			}
			round := Event{Type: TypeArtifact, Content: event.StorageContent()}
			round.NormalizePayload()
			if round.Artifact == nil || round.Artifact.Kind != tc.kind || !round.Delivers() {
				t.Fatalf("round trip = %+v", round)
			}
			switch tc.kind {
			case PresentationArtifact:
				if round.Artifact.Artifact.Title != "Map" || string(round.Artifact.Artifact.Data) != `{"x":1}` {
					t.Fatalf("inline artifact lost content: %+v", round.Artifact)
				}
			case PresentationPluginView:
				if round.Artifact.View.Reference != tc.reference || round.Artifact.View.File != tc.file || round.Artifact.View.WidgetCode != "<p></p>" {
					t.Fatalf("plugin view lost source: %+v", round.Artifact)
				}
			case PresentationMap:
				if place := round.Artifact.Map.Places[0]; round.Artifact.Map.Title != "Coffee" || place.Name != "Caravan" || place.Latitude != 51.5357 || place.Reviews != 3100 {
					t.Fatalf("map lost places: %+v", round.Artifact.Map)
				}
			case PresentationCanvas:
				if round.Artifact.View != nil || round.Artifact.Artifact != nil || strings.Contains(string(wire), `"file"`) {
					t.Fatalf("canvas has unrelated payload: %s", wire)
				}
			}
		})
	}
}
