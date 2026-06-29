package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestUploadAttachmentStoresServerFile(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "attachments", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	handler := (&Server{Store: store, Workspace: workspace}).Handler()

	attachment := uploadTestAttachment(t, handler, session.ID, "../bad name?.txt", "hello attachment")

	if attachment.ID == "" || attachment.Name != "bad name-.txt" || attachment.Size != int64(len("hello attachment")) {
		t.Fatalf("attachment metadata = %#v", attachment)
	}
	if !strings.HasPrefix(attachment.URI, "file://") {
		t.Fatalf("uri = %q, want file://", attachment.URI)
	}
	if !strings.HasPrefix(attachment.ServerPath, filepath.Join(workspace, ".jaz-attachments", session.ID)) {
		t.Fatalf("server path %q outside attachment dir", attachment.ServerPath)
	}
	data, err := os.ReadFile(attachment.ServerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello attachment" {
		t.Fatalf("stored attachment = %q", data)
	}
	metadata, err := readAttachmentMetadata(filepath.Join(workspace, ".jaz-attachments", session.ID), attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URI != "" || metadata.ServerPath == "" {
		t.Fatalf("stored metadata should keep server path without derived uri, got %#v", metadata)
	}
}

func TestACPStreamResolvesAttachmentIDs(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "acp-attachments",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	handler := (&Server{Store: store, ACP: manager, Workspace: workspace}).Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "note.txt", "hello")

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"read this","attachment_ids":["`+attachment.ID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(manager.sent.Attachments) != 1 {
		t.Fatalf("attachments = %#v", manager.sent.Attachments)
	}
	got := manager.sent.Attachments[0]
	if got.ID != attachment.ID || got.Name != "note.txt" || got.URI != "" || got.ServerPath != attachment.ServerPath {
		t.Fatalf("send attachment = %#v, want %#v", got, attachment)
	}
}

func TestACPStreamAllowsAttachmentWithoutText(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "acp-attachment-only",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	handler := (&Server{Store: store, ACP: manager, Workspace: workspace}).Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "image.png", "image-bytes")

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/sessions/"+session.ID+"/messages:stream",
		strings.NewReader(`{"attachment_ids":["`+attachment.ID+`"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if manager.sent.Message != "" {
		t.Fatalf("message = %q, want empty", manager.sent.Message)
	}
	if len(manager.sent.Attachments) != 1 || manager.sent.Attachments[0].ID != attachment.ID {
		t.Fatalf("attachments = %#v", manager.sent.Attachments)
	}
}

func TestStreamDoesNotTrustAttachmentMetadataServerPath(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "acp-attachments",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}
	handler := (&Server{Store: store, ACP: manager, Workspace: workspace}).Handler()
	attachment := uploadTestAttachment(t, handler, session.ID, "note.txt", "hello")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("not this file"), 0o600); err != nil {
		t.Fatal(err)
	}
	poisoned := attachment
	poisoned.ServerPath = outside
	poisoned.URI = filepathx.FileURI(outside)
	if err := writeAttachmentMetadata(filepath.Join(workspace, ".jaz-attachments", session.ID), poisoned); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"read this","attachment_ids":["`+attachment.ID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if len(manager.sent.Attachments) != 1 {
		t.Fatalf("attachments = %#v", manager.sent.Attachments)
	}
	got := manager.sent.Attachments[0]
	if got.ServerPath != attachment.ServerPath || got.URI != "" {
		t.Fatalf("resolved attachment = %#v, want original server path %q", got, attachment.ServerPath)
	}
}

func TestStreamRejectsAttachmentFromAnotherSession(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.CreateSession(storage.CreateSession{Slug: "first", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession(storage.CreateSession{Slug: "second", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, Workspace: t.TempDir()}).Handler()
	attachment := uploadTestAttachment(t, handler, first.ID, "secret.txt", "secret")

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+second.ID+"/messages:stream", strings.NewReader(`{"message":"read","attachment_ids":["`+attachment.ID+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "not found for this session") {
		t.Fatalf("body = %s", res.Body.String())
	}
}

func uploadTestAttachment(t *testing.T, handler http.Handler, sessionID, filename, content string) storage.Attachment {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", res.Code, res.Body.String())
	}
	var attachment storage.Attachment
	if err := json.Unmarshal(res.Body.Bytes(), &attachment); err != nil {
		t.Fatal(err)
	}
	return attachment
}
