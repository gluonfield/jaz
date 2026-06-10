package sqlite

import (
	"context"
	"database/sql"
	stdjson "encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/messagedb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) importLegacyJSON() error {
	legacy := s.mirror
	if legacy == nil {
		var err error
		legacy, err = jsonstore.New(s.root)
		if err != nil {
			return err
		}
	}
	sessions, err := legacy.ListSessions(storage.SessionFilter{IncludeChildren: true})
	if err != nil {
		return err
	}
	threadq := threaddb.New(s.db)
	for _, session := range sessions {
		_, err := threadq.GetThreadIDByID(context.Background(), session.ID)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if session.Status == "" {
			session.Status = storage.StatusIdle
		}
		if session.Runtime == "" {
			session.Runtime = storage.RuntimeNative
		}
		slug, err := s.uniqueSlugLocked(session.Slug, session.ID)
		if err != nil {
			return err
		}
		session.Slug = slug
		messages, err := legacy.LoadMessages(session.ID)
		if err != nil {
			return err
		}
		records, err := recordsFromProviderMessages(messages, session.CreatedAt)
		if err != nil {
			return fmt.Errorf("import legacy session %s: %w", session.ID, err)
		}
		tx, err := s.db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		if err := insertSession(tx, session); err != nil {
			_ = tx.Rollback()
			return err
		}
		messageq := messagedb.New(tx)
		for _, record := range records {
			record.ThreadID = session.ID
			if err := insertMessage(messageq, record); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

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
		records, err := s.loadMessageRecordsLocked(id)
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
