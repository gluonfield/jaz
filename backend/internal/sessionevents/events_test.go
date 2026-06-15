package sessionevents

import (
	"strings"
	"testing"
)

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

func TestArtifactStorageContentDoesNotIncludeDebugSize(t *testing.T) {
	event := Event{
		Type: TypeArtifact,
		Artifact: &ArtifactEvent{
			Title:        "Map",
			WidgetCode:   "<svg></svg>",
			ArtifactType: "svg",
		},
	}
	content := event.StorageContent()
	if strings.Contains(content, "bytes") {
		t.Fatalf("artifact storage content must not include debug size: %s", content)
	}
}
