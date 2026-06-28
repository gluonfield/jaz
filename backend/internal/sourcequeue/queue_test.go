package sourcequeue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestQueueReservesCompletesAndKeepsNewerDirtyMark(t *testing.T) {
	root := t.TempDir()
	t1 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	queue := &Queue{Root: root, Now: func() time.Time { return t1 }}
	source := Source{
		Path:      "sources/telegram/personal/conversations/user-1/2026/06/27.md",
		DirtyAt:   t1,
		Provider:  "telegram",
		Kind:      "chat_day",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "user:1", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "personal", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}

	if err := queue.MarkDirtySource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != source.Path || reserved[0].Kind != source.Kind || reserved[0].Provider != source.Provider || reserved[0].Key != source.Key || len(reserved[0].Replay.Scopes) != 1 || !reserved[0].DirtyAt.Equal(t1) {
		t.Fatalf("reserved = %#v", reserved)
	}

	queue.Now = func() time.Time { return t2 }
	if err := queue.MarkDirtySource(context.Background(), Source{Path: source.Path, DirtyAt: t2, Provider: source.Provider, Kind: source.Kind, MediaType: source.MediaType}); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(context.Background(), reserved); err != nil {
		t.Fatal(err)
	}
	reserved, err = queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || !reserved[0].DirtyAt.Equal(t2) {
		t.Fatalf("newer dirty mark was lost: %#v", reserved)
	}
}

func TestQueueReleasesFailedReservations(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return now }}
	source := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", DirtyAt: now, Provider: "gmail", Kind: "email_message", MediaType: "text/markdown"}

	if err := queue.MarkDirtySource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Release(context.Background(), reserved); err != nil {
		t.Fatal(err)
	}
	reserved, err = queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != source.Path || reserved[0].Kind != source.Kind {
		t.Fatalf("released source was not retried: %#v", reserved)
	}
}

func TestQueueSettleCompletesAndReleasesInOneWrite(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return now }}
	completed := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", DirtyAt: now, Provider: "gmail", Kind: "email_message"}
	failed := Source{Path: "sources/gmail/personal/messages/2026/06/27/m2.md", DirtyAt: now, Provider: "gmail", Kind: "email_message"}

	for _, source := range []Source{completed, failed} {
		if err := queue.MarkDirtySource(context.Background(), source); err != nil {
			t.Fatal(err)
		}
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 2 {
		t.Fatalf("reserved = %#v", reserved)
	}
	if err := queue.Settle(context.Background(), []Source{reserved[0]}, []Source{reserved[1]}); err != nil {
		t.Fatal(err)
	}
	next, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].Path != reserved[1].Path {
		t.Fatalf("settled queue = %#v, want failed source only", next)
	}
}

func TestQueueRecoversStaleProcessing(t *testing.T) {
	root := t.TempDir()
	t1 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return t1 }, StaleAfter: time.Minute}
	source := Source{Path: "sources/whatsapp/personal/contacts.md", DirtyAt: t1}

	if err := queue.MarkDirtySource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Reserve(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	queue.Now = func() time.Time { return t1.Add(2 * time.Minute) }
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != source.Path {
		t.Fatalf("stale reservation was not recovered: %#v", reserved)
	}
}

func TestQueueStatsCountsDirtyProcessingAndRecoversStale(t *testing.T) {
	root := t.TempDir()
	t1 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return t1 }, StaleAfter: time.Minute}
	first := Source{Path: "sources/telegram/personal/conversations/user-1/2026/06/27.md", DirtyAt: t1}
	second := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", DirtyAt: t1}

	if err := queue.MarkDirtySource(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := queue.MarkDirtySource(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Reserve(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	stats, err := queue.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Dirty != 1 || stats.Processing != 1 {
		t.Fatalf("stats = %#v, want dirty=1 processing=1", stats)
	}

	queue.Now = func() time.Time { return t1.Add(2 * time.Minute) }
	stats, err = queue.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Dirty != 1 || stats.Processing != 1 {
		t.Fatalf("stats recovered stale reservation: %#v", stats)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 2 {
		t.Fatalf("stale reservation was not recovered by reserve: %#v", reserved)
	}
}

func TestQueueRejectsEscapedPath(t *testing.T) {
	err := (&Queue{Root: t.TempDir()}).MarkDirtySource(context.Background(), Source{Path: "../outside.md"})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

func TestQueueWritesStateUnderMemoryRoot(t *testing.T) {
	root := t.TempDir()
	queue := &Queue{Root: root}
	if err := queue.MarkDirtySource(context.Background(), Source{Path: "sources/x.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "dirty-sources.json")); err != nil {
		t.Fatal(err)
	}
}

func TestQueueOmitsEmptySourceMetadata(t *testing.T) {
	root := t.TempDir()
	queue := &Queue{Root: root}
	if err := queue.MarkDirtySource(context.Background(), Source{Path: "sources/x.md"}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, ".state", "dirty-sources.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, notWant := range []string{`"key":{}`, `"replay":{}`} {
		if strings.Contains(string(data), notWant) {
			t.Fatalf("dirty state contains %s: %s", notWant, data)
		}
	}
	if _, err := queue.Reserve(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, notWant := range []string{`"key":{}`, `"replay":{}`} {
		if strings.Contains(string(data), notWant) {
			t.Fatalf("processing state contains %s: %s", notWant, data)
		}
	}
}

func TestQueueMigratesLegacyDirtyState(t *testing.T) {
	root := t.TempDir()
	dirtyAt := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	statePath := filepath.Join(root, ".state", "dirty-sources.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"dirty":{"sources/gmail/personal/messages/2026/06/27/m1.md":"` + dirtyAt.Format(time.RFC3339Nano) + `"}}`
	if err := os.WriteFile(statePath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	reserved, err := (&Queue{Root: root}).Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != "sources/gmail/personal/messages/2026/06/27/m1.md" || !reserved[0].DirtyAt.Equal(dirtyAt) {
		t.Fatalf("reserved = %#v", reserved)
	}
}

func TestQueueMigratesLegacySourceMetadataToKey(t *testing.T) {
	root := t.TempDir()
	dirtyAt := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	statePath := filepath.Join(root, ".state", "dirty-sources.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":3,"dirty":{"sources/telegram/personal/conversations/user-1/2026/06/27.md":{"dirty_at":"` + dirtyAt.Format(time.RFC3339Nano) + `","provider":"telegram","kind":"chat_day","media_type":"text/markdown","source_id":"user:1","day":"2026-06-27","replay":{"account":"personal","scopes":[{"domain":"messages","day":"2026-06-27"}]}}}}`
	if err := os.WriteFile(statePath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	reserved, err := (&Queue{Root: root}).Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	wantKey := integrations.SourceKey{Entity: "user:1", Day: "2026-06-27"}
	if len(reserved) != 1 || reserved[0].Key != wantKey || len(reserved[0].Replay.Scopes) != 1 {
		t.Fatalf("reserved = %#v", reserved)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "source_id") {
		t.Fatalf("state kept legacy source_id field: %s", data)
	}
}
