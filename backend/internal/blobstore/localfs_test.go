package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLocalFSStorePutGetListDelete(t *testing.T) {
	store, err := NewLocalFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	payload := []byte("hello cloud workspace")
	sum := sha256.Sum256(payload)
	wantHash := hex.EncodeToString(sum[:])

	meta, err := store.Put(ctx, "org-1/run-1/objects/doc.txt", bytes.NewReader(payload), PutOptions{ContentType: "text/plain; charset=utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Key != "org-1/run-1/objects/doc.txt" || meta.Size != int64(len(payload)) || meta.SHA256 != wantHash || meta.ContentType != "text/plain; charset=utf-8" || meta.UpdatedAt.IsZero() {
		t.Fatalf("unexpected put metadata: %+v", meta)
	}

	rc, gotMeta, err := store.Get(ctx, "org-1/run-1/objects/doc.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("got body %q, want %q", data, payload)
	}
	if gotMeta.SHA256 != wantHash || gotMeta.ContentType != "text/plain; charset=utf-8" || gotMeta.Size != int64(len(payload)) {
		t.Fatalf("unexpected get metadata: %+v", gotMeta)
	}

	if _, err := store.Put(ctx, "org-1/plugins/tasks/abc.tar.zst", strings.NewReader("archive"), PutOptions{ContentType: "application/zstd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, "org-2/run-9/objects/other.pdf", strings.NewReader("%PDF-1.7"), PutOptions{}); err != nil {
		t.Fatal(err)
	}

	all, err := store.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Key != "org-1/plugins/tasks/abc.tar.zst" || all[1].Key != "org-1/run-1/objects/doc.txt" || all[2].Key != "org-2/run-9/objects/other.pdf" {
		t.Fatalf("list all = %+v", all)
	}
	if !strings.HasPrefix(all[2].ContentType, "application/pdf") {
		t.Fatalf("expected inferred pdf content type, got %q", all[2].ContentType)
	}

	org1, err := store.List(ctx, "org-1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(org1) != 2 {
		t.Fatalf("list org-1/ = %+v, want 2 items", org1)
	}

	partial, err := store.List(ctx, "org-1/plu")
	if err != nil {
		t.Fatal(err)
	}
	if len(partial) != 1 || partial[0].Key != "org-1/plugins/tasks/abc.tar.zst" {
		t.Fatalf("list partial prefix = %+v", partial)
	}

	if err := store.Delete(ctx, "org-1/run-1/objects/doc.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "org-1/run-1/objects/doc.txt"); err != nil {
		t.Fatalf("idempotent delete failed: %v", err)
	}
	if _, _, err := store.Get(ctx, "org-1/run-1/objects/doc.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted key err = %v, want ErrNotFound", err)
	}
}

func TestLocalFSStoreOverwriteAndMissingMetaFallback(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFSStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	if _, err := store.Put(ctx, "a/file.json", strings.NewReader(`{"v":1}`), PutOptions{ContentType: "application/json"}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Put(ctx, "a/file.json", strings.NewReader(`{"v":22}`), PutOptions{ContentType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Size != 8 {
		t.Fatalf("overwritten size = %d, want 8", updated.Size)
	}

	if err := os.Remove(filepath.Join(root, "meta", "a", "file.json.json")); err != nil {
		t.Fatal(err)
	}
	rc, meta, err := store.Get(ctx, "a/file.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if meta.Size != 8 || !strings.HasPrefix(meta.ContentType, "application/json") || meta.UpdatedAt.IsZero() {
		t.Fatalf("fallback meta = %+v", meta)
	}
}

func TestLocalFSStoreValidationAndPresign(t *testing.T) {
	if _, err := NewLocalFSStore("   "); err == nil {
		t.Fatal("empty root must fail")
	}
	store, err := NewLocalFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	badKeys := []string{"", "/", "/leading", "trailing/", "../escape", "a/../b", "a/./b", "a//b", `a\b`, "a/\x00b"}
	for _, key := range badKeys {
		if _, err := store.Put(ctx, key, strings.NewReader("x"), PutOptions{}); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Put(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if _, _, err := store.Get(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Get(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if err := store.Delete(ctx, key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Delete(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.PresignGet(ctx, key, time.Minute); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("PresignGet(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if _, err := store.PresignPut(ctx, key, PutOptions{}, time.Minute); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("PresignPut(%q) err = %v, want ErrInvalidKey", key, err)
		}
	}

	if _, err := store.List(ctx, "../escape"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("List traversal err = %v, want ErrInvalidKey", err)
	}
	if _, err := store.PresignGet(ctx, "ok/key", 0); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("PresignGet zero ttl err = %v, want ErrInvalidTTL", err)
	}
	if _, err := store.PresignPut(ctx, "ok/key", PutOptions{}, -time.Second); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("PresignPut negative ttl err = %v, want ErrInvalidTTL", err)
	}
	if _, err := store.PresignGet(ctx, "ok/key", time.Minute); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignGet err = %v, want ErrPresignUnsupported", err)
	}
	if _, err := store.PresignPut(ctx, "ok/key", PutOptions{}, time.Minute); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignPut err = %v, want ErrPresignUnsupported", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Put(canceled, "ok/key", strings.NewReader("x"), PutOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Put err = %v, want context.Canceled", err)
	}
	if missing, err := store.List(ctx, "no-such-org/run-1/"); err != nil || len(missing) != 0 {
		t.Fatalf("List missing subdir = (%+v, %v), want empty", missing, err)
	}
}

type cancelAfterFirstRead struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelAfterFirstRead) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == 1 {
		n := copy(p, "chunk-1")
		r.cancel()
		return n, nil
	}
	return copy(p, "chunk-2"), nil
}

func TestLocalFSStoreCancelsMidCopyAndSerializesConcurrentPuts(t *testing.T) {
	store, err := NewLocalFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	midCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := store.Put(midCtx, "cancel/file.bin", &cancelAfterFirstRead{cancel: cancel}, PutOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-copy cancel err = %v, want context.Canceled", err)
	}
	if _, _, err := store.Get(t.Context(), "cancel/file.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canceled Put left object on disk: %v", err)
	}

	const key = "shared/concurrent.txt"
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := range 6 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 15 {
				payload := fmt.Sprintf("writer-%d-iter-%d-%s", id, i, strings.Repeat("x", (id+i)*13))
				if _, err := store.Put(t.Context(), key, strings.NewReader(payload), PutOptions{ContentType: "text/plain"}); err != nil {
					errs <- err
					return
				}
				rc, meta, err := store.Get(t.Context(), key)
				if errors.Is(err, ErrNotFound) {
					continue
				}
				if err != nil {
					errs <- err
					return
				}
				data, readErr := io.ReadAll(rc)
				_ = rc.Close()
				if readErr != nil {
					errs <- readErr
					return
				}
				sum := sha256.Sum256(data)
				if meta.SHA256 != hex.EncodeToString(sum[:]) || meta.Size != int64(len(data)) {
					errs <- fmt.Errorf("torn object/meta: meta=%+v actualLen=%d", meta, len(data))
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
