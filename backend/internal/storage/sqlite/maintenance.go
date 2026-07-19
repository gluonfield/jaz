package sqlite

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) resetStaleRunningThreads() error {
	return threaddb.New(s.db).ResetRunningThreads(context.Background(), threaddb.ResetRunningThreadsParams{
		Status:        storage.StatusError,
		Error:         nullDBString("Server restarted while this thread was still running."),
		UpdatedAtMs:   timeToMs(time.Now().UTC()),
		RunningStatus: storage.StatusRunning,
	})
}

func (s *Store) backfillMissingThreadErrors() error {
	q := threaddb.New(s.db)
	ids, err := q.ListErrorThreadIDsWithoutError(context.Background(), storage.StatusError)
	if err != nil {
		return err
	}

	for _, id := range ids {
		records, err := s.loadMessageRecords(id)
		if err != nil {
			return err
		}
		if message := sessionErrorFromRecords(records); message != "" {
			if err := q.SetThreadError(context.Background(), threaddb.SetThreadErrorParams{
				Error: nullDBString(message),
				ID:    id,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func sessionErrorFromRecords(records []storage.Message) string {
	for i := len(records) - 1; i >= 0; i-- {
		blocks := records[i].Blocks
		for j := len(blocks) - 1; j >= 0; j-- {
			block := blocks[j]
			if block.Type != "tool" || strings.TrimSpace(block.Result) == "" {
				continue
			}
			var parsed struct {
				Error  string `json:"error"`
				Status string `json:"status"`
			}
			if err := stdjson.Unmarshal([]byte(block.Result), &parsed); err != nil {
				continue
			}
			if parsed.Error == "" || (parsed.Status != "" && parsed.Status != storage.StatusError) {
				continue
			}
			if block.Name != "" {
				return fmt.Sprintf("%s failed: %s", block.Name, parsed.Error)
			}
			return parsed.Error
		}
	}
	return ""
}
