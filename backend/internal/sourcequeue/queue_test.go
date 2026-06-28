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

func TestQueueReservesCompletesAndKeepsNewerPendingMark(t *testing.T) {
	root := t.TempDir()
	t1 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	queue := &Queue{Root: root, Now: func() time.Time { return t1 }}
	source := Source{
		Path:      "sources/telegram/personal/conversations/user-1/2026/06/27.md",
		PendingAt: t1,
		Provider:  "telegram",
		Kind:      "chat_day",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "user:1", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "personal", Scopes: []integrations.ReplayScope{{Domain: integrations.RecordDomainMessages, Day: "2026-06-27"}}},
	}

	if err := queue.MarkPendingSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	reserved, err := queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Path != source.Path || reserved[0].Kind != source.Kind || reserved[0].Provider != source.Provider || reserved[0].Key != source.Key || len(reserved[0].Replay.Scopes) != 1 || !reserved[0].PendingAt.Equal(t1) {
		t.Fatalf("reserved = %#v", reserved)
	}

	queue.Now = func() time.Time { return t2 }
	if err := queue.MarkPendingSource(context.Background(), Source{Path: source.Path, PendingAt: t2, Provider: source.Provider, Kind: source.Kind, MediaType: source.MediaType}); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(context.Background(), reserved); err != nil {
		t.Fatal(err)
	}
	reserved, err = queue.Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || !reserved[0].PendingAt.Equal(t2) {
		t.Fatalf("newer pending mark was lost: %#v", reserved)
	}
}

func TestQueueReleasesFailedReservations(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return now }}
	source := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", PendingAt: now, Provider: "gmail", Kind: "email_message", MediaType: "text/markdown"}

	if err := queue.MarkPendingSource(context.Background(), source); err != nil {
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
	completed := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", PendingAt: now, Provider: "gmail", Kind: "email_message"}
	failed := Source{Path: "sources/gmail/personal/messages/2026/06/27/m2.md", PendingAt: now, Provider: "gmail", Kind: "email_message"}

	for _, source := range []Source{completed, failed} {
		if err := queue.MarkPendingSource(context.Background(), source); err != nil {
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
	source := Source{Path: "sources/whatsapp/personal/contacts.md", PendingAt: t1}

	if err := queue.MarkPendingSource(context.Background(), source); err != nil {
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

func TestQueueStatsCountsPendingProcessingAndRecoversStale(t *testing.T) {
	root := t.TempDir()
	t1 := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	queue := &Queue{Root: root, Now: func() time.Time { return t1 }, StaleAfter: time.Minute}
	first := Source{Path: "sources/telegram/personal/conversations/user-1/2026/06/27.md", PendingAt: t1}
	second := Source{Path: "sources/gmail/personal/messages/2026/06/27/m1.md", PendingAt: t1}

	if err := queue.MarkPendingSource(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := queue.MarkPendingSource(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Reserve(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	stats, err := queue.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pending != 1 || stats.Processing != 1 {
		t.Fatalf("stats = %#v, want pending=1 processing=1", stats)
	}

	queue.Now = func() time.Time { return t1.Add(2 * time.Minute) }
	stats, err = queue.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pending != 1 || stats.Processing != 1 {
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
	err := (&Queue{Root: t.TempDir()}).MarkPendingSource(context.Background(), Source{Path: "../outside.md"})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

func TestQueueWritesStateUnderMemoryRoot(t *testing.T) {
	root := t.TempDir()
	queue := &Queue{Root: root}
	if err := queue.MarkPendingSource(context.Background(), Source{Path: "sources/x.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "pending-sources.json")); err != nil {
		t.Fatal(err)
	}
}

func TestQueueOmitsEmptySourceMetadata(t *testing.T) {
	root := t.TempDir()
	queue := &Queue{Root: root}
	if err := queue.MarkPendingSource(context.Background(), Source{Path: "sources/x.md"}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, ".state", "pending-sources.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, notWant := range []string{`"key":{}`, `"replay":{}`} {
		if strings.Contains(string(data), notWant) {
			t.Fatalf("pending state contains %s: %s", notWant, data)
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

func TestQueueRoundTripsSourceMetadataThroughState(t *testing.T) {
	root := t.TempDir()
	pendingAt := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	source := Source{
		Path:      "sources/telegram/personal/conversations/user-1/2026/06/27.md",
		PendingAt: pendingAt,
		Provider:  "telegram",
		Kind:      "chat_day",
		MediaType: "text/markdown",
		Key:       integrations.SourceKey{Entity: "user:1", Day: "2026-06-27"},
		Replay:    integrations.Replay{Account: "personal", Scopes: []integrations.ReplayScope{{Domain: "messages", Day: "2026-06-27"}}},
	}
	if err := (&Queue{Root: root}).MarkPendingSource(context.Background(), source); err != nil {
		t.Fatal(err)
	}

	reserved, err := (&Queue{Root: root}).Reserve(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 1 || reserved[0].Key != source.Key || len(reserved[0].Replay.Scopes) != 1 || !reserved[0].PendingAt.Equal(pendingAt) {
		t.Fatalf("reserved = %#v", reserved)
	}
}
