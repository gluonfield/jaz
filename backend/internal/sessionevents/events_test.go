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

func TestNormalizeSideChatPayloadClearsStorageContent(t *testing.T) {
	event := Event{
		Type:    TypeSideChatMessage,
		Content: `{"id":"side-1","role":"assistant","content":"answer"}`,
	}
	event.NormalizePayload()
	if event.SideChat == nil || event.SideChat.ID != "side-1" || event.SideChat.Content != "answer" {
		t.Fatalf("side chat = %#v", event.SideChat)
	}
	if event.Content != "" {
		t.Fatalf("content leaked storage payload: %q", event.Content)
	}
}

func TestSideChatStorageContentUsesTypedPayload(t *testing.T) {
	event := Event{
		Type: TypeSideChatMessage,
		SideChat: &SideChatEvent{
			ID:      "side-1",
			Role:    "assistant",
			Content: "answer",
		},
	}
	content := event.StorageContent()
	if !strings.Contains(content, `"id":"side-1"`) || !strings.Contains(content, `"content":"answer"`) {
		t.Fatalf("side chat storage content = %s", content)
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
