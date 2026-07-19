package sessionevents

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestSideChatStorageContentRoundTripsPayload(t *testing.T) {
	event := Event{
		Type: TypeSideChatMessage,
		SideChat: &SideChatEvent{
			ID:      "side-1",
			Role:    "assistant",
			Content: "side answer",
		},
	}

	stored := Event{Type: TypeSideChatMessage, Content: event.StorageContent()}
	stored.NormalizePayload()

	if stored.Content != "" || stored.SideChat == nil {
		t.Fatalf("stored side chat = %#v", stored)
	}
	if stored.SideChat.ID != "side-1" || stored.SideChat.Content != "side answer" {
		t.Fatalf("side chat payload = %#v", stored.SideChat)
	}
}

func TestACPEventSlimForStorageDropsRepeatedMetadata(t *testing.T) {
	event := &ACPEvent{
		ID:              "child",
		Slug:            "review-thread",
		Title:           "Review thread",
		Agent:           "claude",
		SessionID:       "claude-session",
		ModelProvider:   "claude",
		Model:           "claude-opus-4-8",
		ReasoningEffort: "xhigh",
		State:           "running",
		Modes: ACPModeState{
			AvailableModes: []ACPMode{{ID: "default", Name: "Default"}},
		},
	}

	slim := event.SlimForStorage()

	if !(&Event{Type: "acp", ACP: event}).NeedsStorageSlimming() {
		t.Fatal("repeated metadata was not recognized as storage debt")
	}
	if slim.Title != "" || slim.ModelProvider != "" || slim.Model != "" || slim.ReasoningEffort != "" {
		t.Fatalf("slim metadata = %#v", slim)
	}
	if slim.Slug != event.Slug || slim.Agent != event.Agent || slim.SessionID != event.SessionID {
		t.Fatalf("slim identity = %#v", slim)
	}
	if (&Event{Type: "acp", ACP: slim}).NeedsStorageSlimming() {
		t.Fatalf("slim event still reports storage debt: %#v", slim)
	}
}

func TestBusPublishesToEverySessionSubscriber(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := bus.Subscribe(ctx, "session-1")
	second := bus.Subscribe(ctx, "session-1")

	bus.Publish(Event{SessionID: "session-1", Type: "acp_message", Content: "hi"})

	expectBusEvent(t, first, "hi")
	expectBusEvent(t, second, "hi")
}

func TestBusClosesSlowSubscriber(t *testing.T) {
	bus := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := bus.Subscribe(ctx, "session-1")

	for i := 0; i < subscriberBuffer+1; i++ {
		bus.Publish(Event{SessionID: "session-1", Type: "acp_message", Seq: int64(i + 1)})
	}

	for i := 0; i < subscriberBuffer; i++ {
		select {
		case _, ok := <-sub:
			if !ok {
				t.Fatalf("subscriber closed after %d events, want %d buffered events first", i, subscriberBuffer)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for buffered event")
		}
	}
	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("slow subscriber stayed open")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for slow subscriber close")
	}
}

func expectBusEvent(t *testing.T, ch <-chan Event, content string) {
	t.Helper()
	select {
	case event := <-ch:
		if event.Content != content {
			t.Fatalf("event content = %q, want %q", event.Content, content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}
