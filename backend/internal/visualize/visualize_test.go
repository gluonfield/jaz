package visualize

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type fakeEventStore struct {
	events []sessionevents.Event
}

func (f *fakeEventStore) AppendSessionEvents(_ string, events ...sessionevents.Event) error {
	f.events = append(f.events, events...)
	return nil
}

func TestShowWidgetPublishesArtifactEventFromMCPHeader(t *testing.T) {
	store := &fakeEventStore{}
	tools := NewMCPTools(store, nil)
	_, output, err := tools.ShowWidget(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("thread-1")},
	}, ShowWidgetInput{
		LoadingMessages: []string{"Rendering"},
		Title:           "System map",
		WidgetCode:      `<svg width="100%" viewBox="0 0 680 120" role="img"><title>Map</title><desc>System map</desc></svg>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.ArtifactType != "svg" || output.Bytes == 0 {
		t.Fatalf("output = %#v", output)
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}
	event := store.events[0]
	if event.SessionID != "thread-1" || event.Type != sessionevents.TypeArtifact {
		t.Fatalf("event = %#v", event)
	}
	if event.Artifact == nil || event.Artifact.Title != "System map" || event.Artifact.ArtifactType != "svg" {
		t.Fatalf("artifact = %#v", event.Artifact)
	}
}
