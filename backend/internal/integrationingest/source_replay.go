package integrationingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func (p SourceProjector) recordsForSource(source sourcequeue.Source) ([]integrations.Record, error) {
	rawFiles, err := p.rawFilesForSource(source)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []integrations.Record
	for _, file := range rawFiles {
		records, err := readRawRecords(file)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			id := record.ID
			if id == "" {
				id = recordID(record)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := recordTime(out[i])
		right := recordTime(out[j])
		if left.Equal(right) {
			return out[i].ExternalID < out[j].ExternalID
		}
		return left.Before(right)
	})
	return out, nil
}

func (p SourceProjector) rawFilesForSource(source sourcequeue.Source) ([]string, error) {
	root, err := requiredRoot(p.RawRoot)
	if err != nil {
		return nil, err
	}
	provider, err := requiredPathComponent("provider", source.Provider)
	if err != nil {
		return nil, err
	}
	account, err := requiredPathComponent("account", source.Replay.Account)
	if err != nil {
		return nil, err
	}
	if len(source.Replay.Scopes) == 0 {
		return nil, fmt.Errorf("source %q has no raw replay scope", source.Path)
	}
	files := make([]string, 0, len(source.Replay.Scopes))
	for _, scope := range source.Replay.Scopes {
		file, err := rawFileForScope(root, provider, account, scope)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func rawFileForScope(root, provider, account string, scope integrations.ReplayScope) (string, error) {
	switch scope.Domain {
	case integrations.RecordDomainContacts:
		return filepath.Join(root, provider, account, "contacts", "contacts.jsonl"), nil
	case integrations.RecordDomainMessages, integrations.RecordDomainEvents:
		day, err := time.Parse("2006-01-02", scope.Day)
		if err != nil {
			return "", fmt.Errorf("source replay day %q is invalid", scope.Day)
		}
		filename := "events.jsonl"
		if scope.Domain == integrations.RecordDomainMessages {
			filename = "messages.jsonl"
		}
		return filepath.Join(root, provider, account, string(scope.Domain), day.Format("2006"), day.Format("01"), day.Format("02"), filename), nil
	default:
		return "", fmt.Errorf("unsupported source replay domain %q", scope.Domain)
	}
}

func readRawRecords(path string) ([]integrations.Record, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var records []integrations.Record
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record integrations.Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, err
		}
		if record.Kind != "" {
			records = append(records, record)
		}
	}
	return records, scanner.Err()
}
