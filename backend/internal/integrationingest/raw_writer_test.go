package integrationingest

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestRawWriterAppendsJSONLByProviderAndDay(t *testing.T) {
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
	path, err := RawRecordPath(root, integrations.Record{Provider: "gmail", OccurredAt: occurred})
	if err != nil {
		t.Fatal(err)
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

func TestRawWriterRejectsMissingRoot(t *testing.T) {
	err := (RawWriter{}).WriteRecords(context.Background(), []integrations.Record{{Provider: "gmail"}})
	if err == nil {
		t.Fatal("expected error")
	}
}
