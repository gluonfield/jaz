package sessionevents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoredPresentationsNormalizeAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored string
		kind   PresentationKind
		file   string
	}{
		{"inline", `{"presented":true,"title":"Map","widget_code":"<svg></svg>","artifact_type":"svg","data":{"x":1}}`, PresentationArtifact, ""},
		{"plugin", `{"presented":true,"title":"Status","widget_code":"<p></p>","artifact_type":"html","file":"status.json"}`, PresentationPluginView, "status.json"},
		{"canvas", `{"presented":true,"title":"Canvas","widget_code":"","artifact_type":"canvas","file":"/private/runtime/canvases/id/canvas.json"}`, PresentationCanvas, ""},
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
				if round.Artifact.View.File != tc.file || round.Artifact.View.WidgetCode != "<p></p>" {
					t.Fatalf("plugin view lost source: %+v", round.Artifact)
				}
			case PresentationCanvas:
				if round.Artifact.View != nil || round.Artifact.Artifact != nil || strings.Contains(string(wire), `"file"`) {
					t.Fatalf("canvas has unrelated payload: %s", wire)
				}
			}
		})
	}
}
