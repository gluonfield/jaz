package integrationingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type RawWriter struct {
	Root string
	Now  func() time.Time
}

func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".memory", "raw-sources")
	}
	return filepath.Join(home, ".memory", "raw-sources")
}

func (w RawWriter) WriteRecords(ctx context.Context, records []integrations.Record) error {
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.writeRecord(record); err != nil {
			return err
		}
	}
	return nil
}

func (w RawWriter) writeRecord(record integrations.Record) error {
	record = w.prepare(record)
	path, err := RawRecordPath(w.root(), record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	if _, err := writer.Write(append(line, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}

func (w RawWriter) root() string {
	if root := strings.TrimSpace(w.Root); root != "" {
		return root
	}
	return DefaultRoot()
}

func (w RawWriter) prepare(record integrations.Record) integrations.Record {
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	if record.ReceivedAt.IsZero() {
		record.ReceivedAt = now
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = record.ReceivedAt
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = recordID(record)
	}
	return record
}

func recordID(record integrations.Record) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		record.Provider,
		record.ConnectionID,
		record.AccountID,
		string(record.Kind),
		record.ExternalID,
		record.OccurredAt.UTC().Format(time.RFC3339Nano),
		string(record.Raw),
	}, "\x00")))
	return "rec_" + hex.EncodeToString(sum[:12])
}

func RawRecordPath(root string, record integrations.Record) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = DefaultRoot()
	}
	provider, err := requiredPathComponent("provider", record.Provider)
	if err != nil {
		return "", err
	}
	accountID, err := requiredPathComponent("account id", record.AccountID)
	if err != nil {
		return "", err
	}
	connectionID, err := requiredPathComponent("connection id", record.ConnectionID)
	if err != nil {
		return "", err
	}
	domain := record.Kind.Domain()
	if domain == integrations.RecordDomainContacts {
		return filepath.Join(root, provider, accountID, connectionID, string(domain), "contacts.jsonl"), nil
	}
	day := record.OccurredAt
	if day.IsZero() {
		day = record.ReceivedAt
	}
	if day.IsZero() {
		return "", fmt.Errorf("record time is required")
	}
	day = day.UTC()
	filename := "events.jsonl"
	if domain == integrations.RecordDomainMessages {
		filename = "messages.jsonl"
	}
	return filepath.Join(
		root,
		provider,
		accountID,
		connectionID,
		string(domain),
		day.Format("2006"),
		day.Format("01"),
		day.Format("02"),
		filename,
	), nil
}

func requiredPathComponent(name, value string) (string, error) {
	component := integrations.NormalizeAlias(value)
	if component == "" {
		return "", fmt.Errorf("record %s is required", name)
	}
	return component, nil
}
