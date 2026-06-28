package integrationingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestSourceWriterReplacesArtifactsAndMarksMaterializedPathDirty(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	dirty := &fakeDirtySourceStore{}
	writer := SourceWriter{
		Root:             root,
		Now:              func() time.Time { return now },
		DirtySourceStore: dirty,
	}

	err := writer.WriteArtifacts(context.Background(), []integrations.Artifact{{
		Provider:  "telegram",
		Kind:      "chat_day",
		PathHint:  "sources/telegram/personal/conversations/user-1/2026/06/27.md",
		MediaType: "text/markdown",
		Body:      []byte("10:42:09 Me: hello\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = writer.WriteArtifacts(context.Background(), []integrations.Artifact{{
		Provider: "telegram",
		Kind:     "chat_day",
		PathHint: "sources/telegram/personal/conversations/user-1/2026/06/27.md",
		Body:     []byte("10:43:01 Alice: hi"),
	}})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "sources", "telegram", "personal", "conversations", "user-1", "2026", "06", "27.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "10:43:01 Alice: hi\n"; got != want {
		t.Fatalf("source body = %q, want %q", got, want)
	}
	if len(dirty.sources) != 2 || dirty.sources[1].Path != "sources/telegram/personal/conversations/user-1/2026/06/27.md" || !dirty.sources[1].DirtyAt.Equal(now) {
		t.Fatalf("dirty sources = %#v", dirty.sources)
	}
	if dirty.sources[1].Provider != "telegram" {
		t.Fatalf("dirty source lost provider metadata: %#v", dirty.sources[1])
	}
}

func TestSourceWriterRejectsEscapedPathHints(t *testing.T) {
	err := (SourceWriter{Root: t.TempDir()}).WriteArtifacts(context.Background(), []integrations.Artifact{{
		PathHint: "../outside.md",
		Body:     []byte("nope"),
	}})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

type fakeDirtySourceStore struct {
	sources []sourcequeue.Source
}

func (s *fakeDirtySourceStore) MarkDirtySource(_ context.Context, source sourcequeue.Source) error {
	s.sources = append(s.sources, source)
	return nil
}
