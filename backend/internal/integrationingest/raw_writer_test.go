package integrationingest

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestRawWriterAppendsMessagesByProviderAccountConnectionAndDay(t *testing.T) {
	root := t.TempDir()
	occurred := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	writer := RawWriter{Root: root, Now: func() time.Time { return occurred.Add(time.Minute) }}

	err := writer.WriteRecords(context.Background(), []integrations.Record{
		{
			Provider:     "Gmail",
			ConnectionID: "conn_1",
			AccountID:    "august@example.com",
			Kind:         "gmail.message.received",
			ExternalID:   "msg_1",
			OccurredAt:   occurred,
			Raw:          json.RawMessage(`{"subject":"hello"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := integrations.Record{
		Provider:     "gmail",
		ConnectionID: "conn_1",
		AccountID:    "august@example.com",
		Kind:         "gmail.message.received",
		OccurredAt:   occurred,
	}
	path, err := RawRecordPath(root, record)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "gmail", "august-example-com", "conn-1", "messages", "2026", "06", "12", "messages.jsonl")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %o, want 700", got)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected one JSONL record")
	}
	var got integrations.Record
	if err := json.Unmarshal(scanner.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Provider != "Gmail" || got.ReceivedAt.IsZero() {
		t.Fatalf("record = %#v", got)
	}
	if scanner.Scan() {
		t.Fatal("unexpected second record")
	}
}

func TestRawWriterAppendsContactsToStableContactExport(t *testing.T) {
	root := t.TempDir()
	received := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	writer := RawWriter{Root: root, Now: func() time.Time { return received }}

	err := writer.WriteRecords(context.Background(), []integrations.Record{
		{
			Provider:     "telegram",
			ConnectionID: "conn_1",
			AccountID:    "august",
			Kind:         "telegram.contact",
			ExternalID:   "+447700900123",
			Raw:          json.RawMessage(`{"name":"Alice","phone":"+447700900123"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := RawRecordPath(root, integrations.Record{
		Provider:     "telegram",
		ConnectionID: "conn_1",
		AccountID:    "august",
		Kind:         "telegram.contact",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "telegram", "august", "conn-1", "contacts", "contacts.jsonl")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("file mode = %s, want %s", got, want)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("dir mode = %s, want %s", got, want)
	}
}

func TestRawWriterWritesAttachmentFileByProviderAccountConnectionAndIDs(t *testing.T) {
	root := t.TempDir()
	path, err := (RawWriter{Root: root}).WriteAttachment(context.Background(), RawAttachment{
		Provider:     "gmail",
		AccountID:    "august@example.com",
		ConnectionID: "gmail:august",
		MessageID:    "msg/1",
		AttachmentID: "att/1",
		FileName:     "../Report Final.PDF",
		Data:         []byte("attachment bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "gmail/august-example-com/gmail-august/attachments/msg-1-") || !strings.Contains(rel, "/att-1-") || filepath.Base(path) != "report-final.pdf" {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "attachment bytes" {
		t.Fatalf("data = %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("file mode = %s, want %s", got, want)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("dir mode = %s, want %s", got, want)
	}
}

func TestRawAttachmentPathDoesNotCollapseDistinctExternalIDs(t *testing.T) {
	root := t.TempDir()
	first, err := RawAttachmentPath(root, RawAttachment{
		Provider:     "gmail",
		AccountID:    "august@example.com",
		ConnectionID: "gmail:august",
		MessageID:    "msg/1",
		AttachmentID: "att/1",
		FileName:     "report.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RawAttachmentPath(root, RawAttachment{
		Provider:     "gmail",
		AccountID:    "august@example.com",
		ConnectionID: "gmail:august",
		MessageID:    "msg_1",
		AttachmentID: "att_1",
		FileName:     "report.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("paths collapsed: %q", first)
	}
}

func TestRawWriterKeepsAllArchiveDirectoriesPrivate(t *testing.T) {
	root := t.TempDir()
	occurred := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	pathDirs := []string{
		root,
		filepath.Join(root, "telegram"),
		filepath.Join(root, "telegram", "august"),
		filepath.Join(root, "telegram", "august", "conn-1"),
		filepath.Join(root, "telegram", "august", "conn-1", "messages"),
		filepath.Join(root, "telegram", "august", "conn-1", "messages", "2026"),
		filepath.Join(root, "telegram", "august", "conn-1", "messages", "2026", "06"),
		filepath.Join(root, "telegram", "august", "conn-1", "messages", "2026", "06", "12"),
	}
	for _, dir := range pathDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	err := (RawWriter{Root: root}).WriteRecords(context.Background(), []integrations.Record{{
		Provider:     "telegram",
		ConnectionID: "conn_1",
		AccountID:    "august",
		Kind:         "telegram.message",
		ExternalID:   "msg_1",
		OccurredAt:   occurred,
		Raw:          json.RawMessage(`{"text":"hello"}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range pathDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
			t.Fatalf("%s mode = %s, want %s", dir, got, want)
		}
	}
}

func TestRawWriterRequiresRoot(t *testing.T) {
	occurred := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)

	_, err := RawRecordPath("", integrations.Record{
		Provider:     "gmail",
		ConnectionID: "conn_1",
		AccountID:    "august@example.com",
		OccurredAt:   occurred,
	})
	if err == nil {
		t.Fatal("expected missing root error")
	}
	err = (RawWriter{}).WriteRecords(context.Background(), []integrations.Record{{
		Provider:     "gmail",
		ConnectionID: "conn_1",
		AccountID:    "august@example.com",
		OccurredAt:   occurred,
	}})
	if err == nil {
		t.Fatal("expected writer missing root error")
	}
}

func TestRawWriterRejectsMissingPathKeys(t *testing.T) {
	root := t.TempDir()
	tests := []integrations.Record{
		{AccountID: "august@example.com", ConnectionID: "conn_1"},
		{Provider: "gmail", ConnectionID: "conn_1"},
		{Provider: "gmail", AccountID: "august@example.com"},
		{Provider: "---", AccountID: "august@example.com", ConnectionID: "conn_1"},
		{Provider: "gmail", AccountID: "---", ConnectionID: "conn_1"},
		{Provider: "gmail", AccountID: "august@example.com", ConnectionID: "---"},
	}
	for _, record := range tests {
		err := (RawWriter{Root: root}).WriteRecords(context.Background(), []integrations.Record{record})
		if err == nil {
			t.Fatalf("expected error for %#v", record)
		}
	}
}
