package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestLoadTranscriptPageAdvancesMessageAndEventWindowsTogether(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "paged-transcript"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	var messages []storage.Message
	for turn := range 20 {
		at := base.Add(time.Duration(turn) * time.Minute)
		messages = append(messages,
			storage.Message{Role: "user", Content: fmt.Sprintf("prompt-%d", turn), CreatedAt: at},
			storage.Message{Role: "assistant", Content: fmt.Sprintf("answer-%d", turn), CreatedAt: at.Add(time.Second)},
		)
	}
	events := make([]sessionevents.Event, 300)
	for i := range events {
		events[i] = sessionevents.Event{Type: "note", Content: fmt.Sprintf("event-%d", i), At: base.Add(time.Duration(i) * time.Second)}
	}
	if err := store.AppendMessageRecords(session.ID, messages...); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}

	latest, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if !latest.HasEarlier || latest.BeforeMessageSeq != 13 || latest.BeforeEventSeq == 0 || len(latest.Messages) != 28 || len(latest.Events) != transcriptEventPageRows || latest.LatestEventSeq != 300 {
		t.Fatalf("latest page = %#v", latest)
	}
	if latest.Messages[0].Content != "prompt-6" || latest.Events[0].Content != "event-44" {
		t.Fatalf("latest boundary = %#v, %#v", latest.Messages[0], latest.Events[0])
	}

	earlier, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{
		BeforeMessageSeq: latest.BeforeMessageSeq,
		BeforeEventSeq:   latest.BeforeEventSeq,
		HistoryRevision:  latest.HistoryRevision,
		Turns:            24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if earlier.HasEarlier || earlier.BeforeMessageSeq != 0 || earlier.BeforeEventSeq != 0 || len(earlier.Messages) != 12 || len(earlier.Events) != 44 || earlier.LatestEventSeq != 300 {
		t.Fatalf("earlier page = %#v", earlier)
	}
	if earlier.Messages[len(earlier.Messages)-1].Seq >= latest.Messages[0].Seq {
		t.Fatalf("pages overlap: earlier=%d latest=%d", earlier.Messages[len(earlier.Messages)-1].Seq, latest.Messages[0].Seq)
	}
}

func TestLoadTranscriptSessionsBatchesChildrenAndReferences(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	parent, err := store.CreateSession(storage.CreateSession{Slug: "related-parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Slug: "related-child", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	sourcedChild, err := store.CreateSession(storage.CreateSession{
		Slug: "sourced-child", ParentID: parent.ID, SourceType: storage.SourceLoopRun,
	})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := store.CreateSession(storage.CreateSession{Slug: "related-reference"})
	if err != nil {
		t.Fatal(err)
	}
	related, err := store.LoadTranscriptSessions(t.Context(), parent.ID, []string{reference.ID, sourcedChild.ID, "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(related.Children) != 1 || related.Children[0].ID != child.ID {
		t.Fatalf("children = %#v", related.Children)
	}
	referenceIDs := make(map[string]bool, len(related.References))
	for _, item := range related.References {
		referenceIDs[item.ID] = true
	}
	if len(related.References) != 2 || !referenceIDs[sourcedChild.ID] || !referenceIDs[reference.ID] {
		t.Fatalf("references = %#v", related.References)
	}
}

func TestTranscriptCursorRejectsMessageRewrite(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "cursor-messages"})
	if err != nil {
		t.Fatal(err)
	}
	messages := make([]provider.Message, 0, 40)
	for i := range 20 {
		messages = append(messages,
			provider.UserMessage(fmt.Sprintf("prompt-%d", i)),
			provider.AssistantMessage(fmt.Sprintf("answer-%d", i), nil),
		)
	}
	if err := store.SaveMessages(session.ID, messages); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if page.BeforeMessageSeq == 0 {
		t.Fatal("test did not create a continuation cursor")
	}
	if err := store.SaveMessages(session.ID, messages[2:]); err != nil {
		t.Fatal(err)
	}
	_, err = store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{
		BeforeMessageSeq: page.BeforeMessageSeq, HistoryRevision: page.HistoryRevision, Turns: 14,
	})
	if !errors.Is(err, storage.ErrTranscriptChanged) {
		t.Fatalf("continuation error = %v", err)
	}
}

func TestTranscriptCursorRejectsCoalescedEventReplacement(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "cursor-coalesce"})
	if err != nil {
		t.Fatal(err)
	}
	subagent := sessionevents.ProviderSubagentEvent{Provider: "codex", ID: "child", Status: "working"}
	key := sessionevents.ProviderSubagentProjectionKey(session.ID, subagent)
	events := []sessionevents.Event{{
		Type: sessionevents.TypeProviderSubagent, ProviderSubagent: &subagent, ProjectionKey: key,
	}}
	for i := range 299 {
		events = append(events, sessionevents.Event{Type: "note", Content: fmt.Sprintf("event-%d", i)})
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if page.BeforeEventSeq == 0 {
		t.Fatal("test did not create a continuation cursor")
	}
	subagent.Status = "completed"
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeProviderSubagent, ProviderSubagent: &subagent, ProjectionKey: key,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{
		BeforeEventSeq: page.BeforeEventSeq, HistoryRevision: page.HistoryRevision, Turns: 14,
	})
	if !errors.Is(err, storage.ErrTranscriptChanged) {
		t.Fatalf("continuation error = %v", err)
	}
}

func TestLoadTranscriptPagesComposeOneTextRun(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "paged-text-run"})
	if err != nil {
		t.Fatal(err)
	}
	const chunks = 600
	annotator := sessionevents.Annotator{}
	events := make([]sessionevents.Event, chunks)
	for i := range events {
		events[i] = annotator.Annotate(sessionevents.Event{
			SessionID: session.ID, Type: sessionevents.TypeACPMessage, Content: fmt.Sprintf("%03d", i),
			ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:one"},
		})
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}

	request := storage.TranscriptPageRequest{Turns: 14}
	var pages [][]sessionevents.Event
	for {
		page, err := store.LoadTranscriptPage(t.Context(), session.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		pages = append([][]sessionevents.Event{sessionevents.CompactTranscript(page.Events)}, pages...)
		if page.BeforeEventSeq == 0 {
			break
		}
		request.BeforeEventSeq = page.BeforeEventSeq
		request.BeforeMessageSeq = page.BeforeMessageSeq
		request.HistoryRevision = page.HistoryRevision
	}
	var projectedInput []sessionevents.Event
	for _, page := range pages {
		projectedInput = append(projectedInput, page...)
	}
	projected := sessionevents.CompactTranscript(projectedInput)
	if len(projected) != 1 {
		t.Fatalf("projected events = %d", len(projected))
	}
	var want strings.Builder
	for i := range chunks {
		fmt.Fprintf(&want, "%03d", i)
	}
	if projected[0].Content != want.String() {
		t.Fatalf("projected text has %d bytes, want %d", len(projected[0].Content), want.Len())
	}
}

func TestTranscriptCursorRejectsConcurrentCompaction(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "cursor-compaction"})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]sessionevents.Event, 300)
	for i := range events {
		events[i] = sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: "x",
			ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:one"},
		}
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if page.BeforeEventSeq == 0 {
		t.Fatal("test did not create a continuation cursor")
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed == 0 {
		t.Fatalf("compaction = %#v", result)
	}
	_, err = store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{
		BeforeEventSeq: page.BeforeEventSeq, HistoryRevision: page.HistoryRevision, Turns: 14,
	})
	if !errors.Is(err, storage.ErrTranscriptChanged) {
		t.Fatalf("continuation error = %v", err)
	}
}

func TestAppendSessionEventsSplitsLargeTextRows(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "large-text"})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("é", storage.MaxTextEventBytes)
	annotator := sessionevents.Annotator{}
	event := annotator.Annotate(sessionevents.Event{
		SessionID: session.ID, Type: sessionevents.TypeACPMessage, Content: content,
		ACP: &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:large"},
	})
	if err := store.AppendSessionEvents(session.ID, event); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) < 2 {
		t.Fatalf("stored rows = %d", len(stored))
	}
	for _, part := range stored {
		if len(part.Content) > storage.MaxTextEventBytes {
			t.Fatalf("stored text row = %d bytes", len(part.Content))
		}
	}
	projected := sessionevents.CompactTranscript(stored)
	if len(projected) != 1 || projected[0].Content != content {
		t.Fatalf("projected text = %d events, %d bytes", len(projected), len(projected[0].Content))
	}
}

func TestLoadTranscriptPageUsesByteBudget(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "byte-page"})
	if err != nil {
		t.Fatal(err)
	}
	const eventBytes = 512 << 10
	content := strings.Repeat("é", eventBytes/2)
	events := make([]sessionevents.Event, 40)
	for i := range events {
		events[i] = sessionevents.Event{Type: "note", Content: content}
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if page.BeforeEventSeq == 0 || !page.HasEarlier {
		t.Fatalf("page cursors = %#v", page)
	}
	total := 0
	for _, event := range page.Events {
		total += len(event.Content)
	}
	if total > storage.MaxTranscriptEventBytes || len(page.Events) != storage.MaxTranscriptEventBytes/eventBytes {
		t.Fatalf("page = %d events, %d bytes", len(page.Events), total)
	}
}

func TestLoadTranscriptPageAcceptsExistingLargeEvent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "large-event-page"})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", 9<<20)
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "note", Content: content}); err != nil {
		t.Fatal(err)
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("large event page = %d events", len(page.Events))
	}
	if page.Events[0].Content != content {
		t.Fatalf("large event = %d bytes, want %d", len(page.Events[0].Content), len(content))
	}
}

func TestLoadTranscriptPageBoundsMessagesByCompleteTurn(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "message-byte-page"})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", 3<<20)
	for range 3 {
		if err := store.AppendMessageRecords(session.ID,
			storage.Message{Role: "user", Content: content},
			storage.Message{Role: "assistant", Content: content},
		); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasEarlier || page.BeforeMessageSeq != 5 || len(page.Messages) != 2 {
		t.Fatalf("bounded message page = %#v", page)
	}
	for _, message := range page.Messages {
		if message.Seq < page.BeforeMessageSeq {
			t.Fatalf("message %d precedes cursor %d", message.Seq, page.BeforeMessageSeq)
		}
	}
}

func TestAppendSessionEventsRejectsOversizeNonTextRow(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "oversize-row"})
	if err != nil {
		t.Fatal(err)
	}
	err = store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: "note", Content: strings.Repeat("x", storage.MaxSessionEventBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("append error = %v", err)
	}
	stored, loadErr := store.LoadSessionEvents(session.ID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(stored) != 0 {
		t.Fatalf("oversize event persisted: %d", len(stored))
	}
}

func TestLoadTranscriptPageBoundsOversizedTurn(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "oversized-turn"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	if err := store.AppendMessageRecords(session.ID,
		storage.Message{Role: "user", Content: "prompt", CreatedAt: base},
		storage.Message{Role: "assistant", Content: "answer", CreatedAt: base.Add(time.Second)},
	); err != nil {
		t.Fatal(err)
	}
	events := make([]sessionevents.Event, 600)
	for i := range events {
		events[i] = sessionevents.Event{Type: "note", Content: fmt.Sprintf("event-%d", i+1), At: base.Add(time.Duration(i+2) * time.Second)}
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}

	page, err := store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != transcriptEventPageRows || page.BeforeEventSeq == 0 || !page.HasEarlier || len(page.Messages) != 2 {
		t.Fatalf("first page = %#v", page)
	}
	seen := make(map[int64]bool, len(events))
	for {
		for _, event := range page.Events {
			if seen[event.Seq] {
				t.Fatalf("event %d returned twice", event.Seq)
			}
			seen[event.Seq] = true
		}
		if page.BeforeEventSeq == 0 {
			break
		}
		page, err = store.LoadTranscriptPage(t.Context(), session.ID, storage.TranscriptPageRequest{
			BeforeMessageSeq: page.BeforeMessageSeq,
			BeforeEventSeq:   page.BeforeEventSeq,
			HistoryRevision:  page.HistoryRevision,
			Turns:            14,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Messages) != 0 || len(page.Events) > transcriptEventPageRows {
			t.Fatalf("continuation page = %#v", page)
		}
	}
	if len(seen) != len(events) {
		t.Fatalf("loaded %d events, want %d", len(seen), len(events))
	}
}
