package integrationingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/connections/materialize"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestMaterializingWriterWritesObservedRecordsAndQueuesProjection(t *testing.T) {
	rawRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	projection := &fakePendingSourceStore{}
	writer := MaterializingWriter{
		Raw:             RawWriter{Root: rawRoot, Now: func() time.Time { return now }},
		ProjectionQueue: projection,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: fakeSourceProjector{},
		},
	}
	record := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "m1",
		OccurredAt:   now,
		Raw:          json.RawMessage(`{"message":"hello"}`),
	}

	if err := writer.WriteRecords(context.Background(), []integrations.Record{record}); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(rawRoot, "telegram", "acct", "messages", "2026", "06", "27", "messages.jsonl")
	if _, err := os.Stat(rawPath); err != nil {
		t.Fatal(err)
	}
	if len(projection.sources) != 1 || projection.sources[0].Path != "sources/telegram/acct/conversations/test/2026/06/27.md" || projection.sources[0].Kind != "chat_day" || projection.sources[0].Provider != "telegram" || len(projection.sources[0].Replay.Scopes) != 1 {
		t.Fatalf("projection sources = %#v", projection.sources)
	}
}

func TestSourceProjectionRunnerRebuildsSourceFromRawShard(t *testing.T) {
	rawRoot := t.TempDir()
	sourceRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	queue := sourcequeue.New(queueRoot)
	memoryPending := &fakePendingSourceStore{}
	writer := MaterializingWriter{
		Raw:             RawWriter{Root: rawRoot, Now: func() time.Time { return now }},
		ProjectionQueue: queue,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: fakeSourceProjector{},
		},
	}
	first := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "m1",
		OccurredAt:   now,
		Raw:          json.RawMessage(`{"message":"hello"}`),
	}
	second := first
	second.ExternalID = "m2"
	second.OccurredAt = now.Add(time.Second)
	second.Raw = json.RawMessage(`{"message":"again"}`)

	if err := writer.WriteRecords(context.Background(), []integrations.Record{first}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteRecords(context.Background(), []integrations.Record{second}); err != nil {
		t.Fatal(err)
	}
	runner := SourceProjectionRunner{
		Queue: queue,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: fakeSourceProjector{},
		},
		Writer: SourceWriter{
			Root:               sourceRoot,
			PendingSourceStore: memoryPending,
		},
	}
	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	sourcePath := filepath.Join(sourceRoot, "sources", "telegram", "acct", "conversations", "test", "2026", "06", "27.md")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "materialized m1\nmaterialized m2\n" {
		t.Fatalf("source = %q", string(data))
	}
	if len(memoryPending.sources) != 1 || memoryPending.sources[0].Path != "sources/telegram/acct/conversations/test/2026/06/27.md" {
		t.Fatalf("memory pending sources = %#v", memoryPending.sources)
	}
	if memoryPending.sources[0].Provider != "telegram" {
		t.Fatalf("memory pending source lost provider metadata: %#v", memoryPending.sources[0])
	}
}

func TestSourceProjectionRunnerRunUntilIdleDrainsMultipleBatches(t *testing.T) {
	rawRoot := t.TempDir()
	sourceRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	raw := RawWriter{Root: rawRoot, Now: func() time.Time { return now }}
	records := []integrations.Record{
		{
			Provider:     "telegram",
			ConnectionID: "conn",
			AccountID:    "acct",
			Kind:         "telegram.message",
			ExternalID:   "m1",
			OccurredAt:   now,
			Raw:          json.RawMessage(`{"message":"one"}`),
		},
		{
			Provider:     "telegram",
			ConnectionID: "conn",
			AccountID:    "acct",
			Kind:         "telegram.message",
			ExternalID:   "m2",
			OccurredAt:   now.AddDate(0, 0, 1),
			Raw:          json.RawMessage(`{"message":"two"}`),
		},
	}
	if err := raw.WriteRecords(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	queue := sourcequeue.New(queueRoot)
	sources := []sourcequeue.Source{
		{
			Path:      "sources/telegram/acct/conversations/a/2026/06/27.md",
			PendingAt: now,
			Provider:  "telegram",
			Kind:      "chat_day",
			MediaType: "text/markdown",
			Key:       integrations.SourceKey{Entity: "a", Day: "2026-06-27"},
			Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
		},
		{
			Path:      "sources/telegram/acct/conversations/b/2026/06/28.md",
			PendingAt: now,
			Provider:  "telegram",
			Kind:      "chat_day",
			MediaType: "text/markdown",
			Key:       integrations.SourceKey{Entity: "b", Day: "2026-06-28"},
			Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-28"}}},
		},
	}
	for _, source := range sources {
		if err := queue.MarkPendingSource(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	memoryPending := &fakePendingSourceStore{}
	runner := SourceProjectionRunner{
		Queue: queue,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: fakeSourceProjector{},
		},
		Writer: SourceWriter{
			Root:               sourceRoot,
			PendingSourceStore: memoryPending,
		},
		BatchFiles: 1,
	}

	processed, err := runner.RunUntilIdle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 {
		t.Fatalf("processed = %d, want 2", processed)
	}
	if len(memoryPending.sources) != 2 {
		t.Fatalf("memory pending sources = %#v, want 2", memoryPending.sources)
	}
	pending, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("projection queue was not drained: %#v", pending)
	}
}

func TestMaterializingWriterQueuesValidSourcesWhenPlanningRecordFails(t *testing.T) {
	rawRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	projection := &fakePendingSourceStore{}
	writer := MaterializingWriter{
		Raw:             RawWriter{Root: rawRoot, Now: func() time.Time { return now }},
		ProjectionQueue: projection,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: partialFailureProjector{},
		},
	}
	bad := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "bad.message",
		ExternalID:   "bad",
		OccurredAt:   now,
		Raw:          json.RawMessage(`{"message":"bad"}`),
	}
	good := bad
	good.Kind = "telegram.message"
	good.ExternalID = "good"
	good.Raw = json.RawMessage(`{"message":"good"}`)

	if err := writer.WriteRecords(context.Background(), []integrations.Record{bad, good}); err != nil {
		t.Fatal(err)
	}
	if len(projection.sources) != 1 || projection.sources[0].Path != "sources/telegram/acct/conversations/good/2026/06/27.md" || projection.sources[0].Kind != "chat_day" {
		t.Fatalf("projection sources = %#v", projection.sources)
	}
}

func TestSourceProjectionRunnerReleasesSourceWhenProjectorProducesNoArtifact(t *testing.T) {
	rawRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	queue := sourcequeue.New(queueRoot)
	source := sourcequeue.Source{
		Path:      "sources/telegram/acct/conversations/test/2026/06/27.md",
		PendingAt: now,
		Provider:  "telegram",
		Kind:      "chat_day",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "test", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}
	if err := queue.MarkPendingSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	runner := SourceProjectionRunner{
		Queue: queue,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: noArtifactProjector{},
		},
		Writer: SourceWriter{Root: t.TempDir()},
	}

	if _, err := runner.RunOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "produced no artifact") {
		t.Fatalf("err = %v, want no artifact error", err)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != source.Path || reserved[0].Kind != source.Kind {
		t.Fatalf("source was not released with metadata: %#v", reserved)
	}
}

func TestSourceProjectionRunnerCompletesSuccessfulSourcesWhenAnotherSourceFails(t *testing.T) {
	rawRoot := t.TempDir()
	sourceRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 30, 0, 0, time.UTC)
	raw := RawWriter{Root: rawRoot, Now: func() time.Time { return now }}
	good := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "good",
		OccurredAt:   now,
		Raw:          json.RawMessage(`{"message":"good"}`),
	}
	if err := raw.WriteRecords(context.Background(), []integrations.Record{good}); err != nil {
		t.Fatal(err)
	}
	queue := sourcequeue.New(queueRoot)
	goodSource := sourcequeue.Source{
		Path:      "sources/telegram/acct/conversations/good/2026/06/27.md",
		PendingAt: now,
		Provider:  "telegram",
		Kind:      "chat_day",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "good", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}
	badSource := goodSource
	badSource.Path = "sources/telegram/acct/conversations/bad/2026/06/27.md"
	badSource.Key.Entity = "bad"
	for _, source := range []sourcequeue.Source{goodSource, badSource} {
		if err := queue.MarkPendingSource(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	runner := SourceProjectionRunner{
		Queue: queue,
		Projector: SourceProjector{
			RawRoot:   rawRoot,
			Projector: noArtifactForBadProjector{},
		},
		Writer: SourceWriter{Root: sourceRoot},
	}

	processed, err := runner.RunOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("err = %v, want bad source error", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != badSource.Path {
		t.Fatalf("remaining pending sources = %#v", reserved)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, filepath.FromSlash(goodSource.Path))); err != nil {
		t.Fatal(err)
	}
}

func TestPlanRecordsQueuesContactDependencyWhenContactChanges(t *testing.T) {
	now := time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC)
	contactRaw, _ := json.Marshal(map[string]any{
		"id":         1,
		"first_name": "Alice",
	})
	projector := SourceProjector{
		Projector: CompositeSourceProjector(materialize.DefaultSourceProjectors()),
	}
	sources, err := projector.PlanRecords(context.Background(), []integrations.Record{{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.contact",
		ExternalID:   "user:1",
		OccurredAt:   now.Add(time.Minute),
		Raw:          contactRaw,
	}})
	if err != nil {
		t.Fatal(err)
	}
	dependencyPath := ".state/source-dependencies/chat-contact/telegram/acct/" + integrations.SourceSlug("user:1") + ".dep"
	var sawContacts, sawDependency, sawChat bool
	for _, source := range sources {
		if source.Path == "sources/telegram/acct/contacts.md" {
			sawContacts = true
		}
		if source.Path == dependencyPath {
			sawDependency = true
		}
		if source.Path == "sources/telegram/acct/conversations/user/1/2026/06/27.md" {
			sawChat = true
		}
	}
	if !sawContacts || !sawDependency || sawChat {
		t.Fatalf("sources = %#v, want contacts and dependency only", sources)
	}
}

func TestPlanRecordsCompactsContactDependencyIndex(t *testing.T) {
	stateRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC)
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"message": "hello",
		"from_id": "user:1",
		"peer_id": "chat:100",
	})
	message := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "chat:100:7",
		OccurredAt:   now,
		Raw:          messageRaw,
	}
	projector := SourceProjector{
		StateRoot: stateRoot,
		Projector: CompositeSourceProjector(materialize.DefaultSourceProjectors()),
	}
	if _, err := projector.PlanRecords(context.Background(), []integrations.Record{message}); err != nil {
		t.Fatal(err)
	}
	if _, err := projector.PlanRecords(context.Background(), []integrations.Record{message}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateRoot, ".state", "source-dependencies", "chat-contact", "telegram", "acct", integrations.SourceSlug("user:1")+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]contactDependencyEntry
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 {
		t.Fatalf("dependency index = %#v, want one source entry", index)
	}
	for _, entry := range index {
		if entry.Key.Entity != "chat:100" || len(entry.Replay.Scopes) != 2 {
			t.Fatalf("dependency entry lost source metadata: %#v", entry)
		}
	}
}

func TestSourceProjectionRunnerExpandsContactDependencyInProjectionWorker(t *testing.T) {
	rawRoot := t.TempDir()
	stateRoot := t.TempDir()
	sourceRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC)
	raw := RawWriter{Root: rawRoot, Now: func() time.Time { return now }}
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"message": "hello",
		"from_id": "user:1",
		"peer_id": "user:1",
	})
	contactRaw, _ := json.Marshal(map[string]any{
		"id":         1,
		"first_name": "Alice",
	})
	messageRecord := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "user:1:7",
		OccurredAt:   now,
		Raw:          messageRaw,
	}
	contactRecord := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.contact",
		ExternalID:   "user:1",
		OccurredAt:   now.Add(time.Minute),
		Raw:          contactRaw,
	}
	if err := raw.WriteRecords(context.Background(), []integrations.Record{messageRecord, contactRecord}); err != nil {
		t.Fatal(err)
	}
	projector := SourceProjector{
		RawRoot:   rawRoot,
		StateRoot: stateRoot,
		Projector: CompositeSourceProjector(materialize.DefaultSourceProjectors()),
	}
	if _, err := projector.PlanRecords(context.Background(), []integrations.Record{messageRecord}); err != nil {
		t.Fatal(err)
	}
	queue := sourcequeue.New(queueRoot)
	dependency := sourcequeue.Source{
		Path:      ".state/source-dependencies/chat-contact/telegram/acct/" + integrations.SourceSlug("user:1") + ".dep",
		PendingAt: now.Add(time.Minute),
		Kind:      sourceKindContactDependency,
	}
	if err := queue.MarkPendingSource(context.Background(), dependency); err != nil {
		t.Fatal(err)
	}
	memoryPending := &fakePendingSourceStore{}
	runner := SourceProjectionRunner{
		Queue:     queue,
		Projector: projector,
		Writer: SourceWriter{
			Root:               sourceRoot,
			PendingSourceStore: memoryPending,
			Now:                func() time.Time { return now.Add(2 * time.Minute) },
		},
	}
	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	sourcePath := filepath.Join(sourceRoot, "sources", "telegram", "acct", "conversations", "user", "1", "2026", "06", "27.md")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"# Telegram · Alice", "Alice | user:1", "10:42:09 Alice: hello"} {
		if !strings.Contains(body, want) {
			t.Fatalf("source body missing %q:\n%s", want, body)
		}
	}
	if len(memoryPending.sources) != 1 || memoryPending.sources[0].Path != "sources/telegram/acct/conversations/user/1/2026/06/27.md" {
		t.Fatalf("memory pending sources = %#v", memoryPending.sources)
	}
}

func TestSourceProjectionRunnerExpandsContactDependencyForGroupSpeaker(t *testing.T) {
	rawRoot := t.TempDir()
	stateRoot := t.TempDir()
	sourceRoot := t.TempDir()
	queueRoot := t.TempDir()
	now := time.Date(2026, 6, 27, 10, 42, 9, 0, time.UTC)
	raw := RawWriter{Root: rawRoot, Now: func() time.Time { return now }}
	messageRaw, _ := json.Marshal(map[string]any{
		"id":      7,
		"message": "hello group",
		"from_id": "user:1",
		"peer_id": "chat:100",
	})
	contactRaw, _ := json.Marshal(map[string]any{
		"id":         1,
		"first_name": "Alice",
	})
	messageRecord := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.message",
		ExternalID:   "chat:100:7",
		OccurredAt:   now,
		Raw:          messageRaw,
	}
	contactRecord := integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn",
		AccountID:    "acct",
		Kind:         "telegram.contact",
		ExternalID:   "user:1",
		OccurredAt:   now.Add(time.Minute),
		Raw:          contactRaw,
	}
	if err := raw.WriteRecords(context.Background(), []integrations.Record{messageRecord, contactRecord}); err != nil {
		t.Fatal(err)
	}
	projector := SourceProjector{
		RawRoot:   rawRoot,
		StateRoot: stateRoot,
		Projector: CompositeSourceProjector(materialize.DefaultSourceProjectors()),
	}
	if _, err := projector.PlanRecords(context.Background(), []integrations.Record{messageRecord}); err != nil {
		t.Fatal(err)
	}
	queue := sourcequeue.New(queueRoot)
	dependency := sourcequeue.Source{
		Path:      ".state/source-dependencies/chat-contact/telegram/acct/" + integrations.SourceSlug("user:1") + ".dep",
		PendingAt: now.Add(time.Minute),
		Kind:      sourceKindContactDependency,
	}
	if err := queue.MarkPendingSource(context.Background(), dependency); err != nil {
		t.Fatal(err)
	}
	memoryPending := &fakePendingSourceStore{}
	runner := SourceProjectionRunner{
		Queue:     queue,
		Projector: projector,
		Writer: SourceWriter{
			Root:               sourceRoot,
			PendingSourceStore: memoryPending,
			Now:                func() time.Time { return now.Add(2 * time.Minute) },
		},
	}
	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	sourcePath := filepath.Join(sourceRoot, "sources", "telegram", "acct", "conversations", "chat", "100", "2026", "06", "27.md")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"# Telegram · chat:100", "Alice | user:1", "10:42:09 Alice: hello group"} {
		if !strings.Contains(body, want) {
			t.Fatalf("source body missing %q:\n%s", want, body)
		}
	}
	if len(memoryPending.sources) != 1 || memoryPending.sources[0].Path != "sources/telegram/acct/conversations/chat/100/2026/06/27.md" {
		t.Fatalf("memory pending sources = %#v", memoryPending.sources)
	}
}

type fakeSourceProjector struct{}

func (fakeSourceProjector) SourceTargets(context.Context, integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	return []integrations.SourceTarget{{
		Provider:  "telegram",
		Kind:      "chat_day",
		PathHint:  "sources/telegram/acct/conversations/test/2026/06/27.md",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "test", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}}, nil
}

func (fakeSourceProjector) ProjectSource(_ context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	var b strings.Builder
	for _, record := range req.Records {
		if record.Kind == "telegram.message" {
			b.WriteString("materialized " + record.ExternalID + "\n")
		}
	}
	return integrations.Artifact{Provider: req.Target.Provider, Kind: req.Target.Kind, PathHint: req.Target.PathHint, MediaType: req.Target.MediaType, Body: []byte(b.String())}, nil
}

type partialFailureProjector struct{}

func (partialFailureProjector) SourceTargets(_ context.Context, req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	if req.Record.ExternalID == "bad" {
		return nil, errors.New("bad record")
	}
	return []integrations.SourceTarget{{
		Provider:  "telegram",
		Kind:      "chat_day",
		PathHint:  "sources/telegram/acct/conversations/good/2026/06/27.md",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "good", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "acct", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}}, nil
}

func (partialFailureProjector) ProjectSource(context.Context, integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	return integrations.Artifact{}, nil
}

type noArtifactProjector struct{}

func (noArtifactProjector) SourceTargets(context.Context, integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	return nil, nil
}

func (noArtifactProjector) ProjectSource(context.Context, integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	return integrations.Artifact{}, nil
}

type noArtifactForBadProjector struct{}

func (noArtifactForBadProjector) SourceTargets(context.Context, integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	return nil, nil
}

func (noArtifactForBadProjector) ProjectSource(_ context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	if req.Target.Key.Entity == "bad" {
		return integrations.Artifact{}, nil
	}
	return fakeSourceProjector{}.ProjectSource(context.Background(), req)
}

var _ PendingSourceStore = (*fakePendingSourceStore)(nil)
