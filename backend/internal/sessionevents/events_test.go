package sessionevents

import "testing"

func TestNormalizeArtifactPayloadClearsStorageContent(t *testing.T) {
	event := Event{
		Type:    TypeArtifact,
		Content: `{"title":"Map","widget_code":"<svg></svg>","artifact_type":"svg"}`,
	}
	event.NormalizePayload()
	if event.Artifact == nil || event.Artifact.Title != "Map" {
		t.Fatalf("artifact = %#v", event.Artifact)
	}
	if event.Content != "" {
		t.Fatalf("content leaked storage payload: %q", event.Content)
	}
}
