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

type RawAttachment struct {
	Provider     string
	AccountID    string
	ConnectionID string
	MessageID    string
	AttachmentID string
	FileName     string
	Data         []byte
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

func (w RawWriter) WriteAttachment(ctx context.Context, attachment RawAttachment) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := w.root()
	if err != nil {
		return "", err
	}
	path, err := RawAttachmentPath(root, attachment)
	if err != nil {
		return "", err
	}
	if err := ensurePrivateDir(root, filepath.Dir(path)); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, attachment.Data, 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (w RawWriter) writeRecord(record integrations.Record) error {
	root, err := w.root()
	if err != nil {
		return err
	}
	record = w.prepare(record)
	path, err := RawRecordPath(root, record)
	if err != nil {
		return err
	}
	return appendJSONLine(root, path, record)
}

func appendJSONLine(root, path string, value any) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDir(root, dir); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}

	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	if _, err := writer.Write(append(line, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}

func (w RawWriter) root() (string, error) {
	return requiredRoot(w.Root)
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
	root, err := requiredRoot(root)
	if err != nil {
		return "", err
	}
	provider, err := requiredPathComponent("provider", record.Provider)
	if err != nil {
		return "", err
	}
	accountID, err := requiredPathComponent("account id", record.AccountID)
	if err != nil {
		return "", err
	}
	if _, err := requiredPathComponent("connection id", record.ConnectionID); err != nil {
		return "", err
	}
	domain := record.Kind.Domain()
	if domain == integrations.RecordDomainContacts {
		return filepath.Join(root, provider, accountID, string(domain), "contacts.jsonl"), nil
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
		string(domain),
		day.Format("2006"),
		day.Format("01"),
		day.Format("02"),
		filename,
	), nil
}

func RawAttachmentPath(root string, attachment RawAttachment) (string, error) {
	root, err := requiredRoot(root)
	if err != nil {
		return "", err
	}
	provider, err := requiredPathComponent("provider", attachment.Provider)
	if err != nil {
		return "", err
	}
	accountID, err := requiredPathComponent("account id", attachment.AccountID)
	if err != nil {
		return "", err
	}
	if _, err := requiredPathComponent("connection id", attachment.ConnectionID); err != nil {
		return "", err
	}
	messageID, err := externalIDPathComponent("message id", attachment.MessageID)
	if err != nil {
		return "", err
	}
	attachmentID, err := externalIDPathComponent("attachment id", attachment.AttachmentID)
	if err != nil {
		return "", err
	}
	return filepath.Join(
		root,
		provider,
		accountID,
		"attachments",
		messageID,
		attachmentID,
		safeAttachmentFileName(attachment.FileName),
	), nil
}

func requiredRoot(value string) (string, error) {
	root := strings.TrimSpace(value)
	if root == "" {
		return "", fmt.Errorf("raw ingest root is required")
	}
	return root, nil
}

func requiredPathComponent(name, value string) (string, error) {
	component := integrations.NormalizeAlias(value)
	if component == "" {
		return "", fmt.Errorf("record %s is required", name)
	}
	return component, nil
}

func externalIDPathComponent(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("record %s is required", name)
	}
	prefix := integrations.NormalizeAlias(value)
	if prefix == "" {
		prefix = "id"
	}
	if len(prefix) > 48 {
		prefix = strings.Trim(prefix[:48], "-")
	}
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:4]), nil
}

func safeAttachmentFileName(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	if name == "" || name == "." || name == string(os.PathSeparator) {
		name = "attachment"
	}
	ext := safeAttachmentExtension(filepath.Ext(name))
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = integrations.NormalizeAlias(stem)
	if stem == "" {
		stem = "attachment"
	}
	return stem + ext
}

func safeAttachmentExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, ".") || len(value) > 17 {
		return ""
	}
	for _, r := range value[1:] {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return value
}

func ensurePrivateDir(root, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		return err
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("raw record path escapes root")
	}
	current := absRoot
	if err := os.Chmod(current, 0o700); err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	for _, component := range strings.Split(rel, string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := os.Chmod(current, 0o700); err != nil {
			return err
		}
	}
	return nil
}
