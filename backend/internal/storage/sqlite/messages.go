package sqlite

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/messagedb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) LoadMessages(id string) ([]provider.Message, error) {
	records, err := s.LoadMessageRecords(id)
	if err != nil {
		return nil, err
	}
	return providerMessagesFromRecords(records)
}

func (s *Store) LoadMessageRecords(id string) ([]storage.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadMessageRecordsLocked(id)
}

func (s *Store) SaveMessages(id string, messages []provider.Message) error {
	records, err := recordsFromProviderMessages(messages, time.Now().UTC())
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.replaceMessagesLocked(id, records)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.mirrorMessages(id, messages)
	return nil
}

func (s *Store) SaveMessagesWithReasoning(id string, messages []provider.Message, reasoningByMessage map[int]string) error {
	return s.SaveMessagesWithReasoningAndMedia(id, messages, reasoningByMessage, nil)
}

func (s *Store) SaveMessagesWithReasoningAndMedia(id string, messages []provider.Message, reasoningByMessage map[int]string, mediaRefs map[string][]media.Ref) error {
	records, err := recordsFromProviderMessagesWithReasoningAndMedia(messages, reasoningByMessage, mediaRefs, time.Now().UTC())
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.replaceMessagesLocked(id, records)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.mirrorMessages(id, messages)
	return nil
}

func (s *Store) AppendMessages(id string, messages ...provider.Message) error {
	if len(messages) == 0 {
		return nil
	}
	now := time.Now().UTC()
	next, err := recordsFromProviderMessages(messages, now)
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.appendMessageRecordsLocked(id, next, now)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if s.mirror != nil {
		_ = s.mirror.AppendMessages(id, messages...)
	}
	return nil
}

func (s *Store) AppendMessageRecords(id string, records ...storage.Message) error {
	if len(records) == 0 {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	err := s.appendMessageRecordsLocked(id, records, now)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if s.mirror != nil {
		if messages, err := providerMessagesFromRecords(records); err == nil {
			_ = s.mirror.AppendMessages(id, messages...)
		}
	}
	return nil
}

func (s *Store) appendMessageRecordsLocked(id string, records []storage.Message, now time.Time) error {
	existing, err := s.loadMessageRecordsLocked(id)
	if err != nil {
		return err
	}
	for i := range records {
		records[i].ThreadID = id
		records[i].Seq = int64(len(existing) + i + 1)
		if records[i].CreatedAt.IsZero() {
			records[i].CreatedAt = now.Add(time.Duration(i+1) * time.Millisecond)
		}
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	messageq := messagedb.New(tx)
	threadq := threaddb.New(tx)
	for _, record := range records {
		if err := insertMessage(messageq, record); err != nil {
			return err
		}
	}
	if err := threadq.TouchThread(context.Background(), threaddb.TouchThreadParams{UpdatedAtMs: timeToMs(now), ID: id}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) loadMessageRecordsLocked(id string) ([]storage.Message, error) {
	rows, err := messagedb.New(s.db).ListMessagesByThread(context.Background(), id)
	if err != nil {
		return nil, err
	}
	records := make([]storage.Message, 0, len(rows))
	for _, row := range rows {
		record, err := messageFromDB(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Store) replaceMessagesLocked(id string, records []storage.Message) error {
	existing, err := s.loadMessageRecordsLocked(id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	messageq := messagedb.New(tx)
	threadq := threaddb.New(tx)
	if err := messageq.DeleteMessagesByThread(context.Background(), id); err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, record := range records {
		record.ThreadID = id
		record.Seq = int64(i + 1)
		// Already-stored rows keep their timestamps; only new rows are stamped.
		if i < len(existing) && existing[i].Role == record.Role {
			record.CreatedAt = existing[i].CreatedAt
			record = storage.MergeDurableBlocks(record, existing[i])
		} else if record.CreatedAt.IsZero() {
			record.CreatedAt = now.Add(time.Duration(i+1) * time.Millisecond)
		}
		if err := insertMessage(messageq, record); err != nil {
			return err
		}
	}
	if err := threadq.TouchThread(context.Background(), threaddb.TouchThreadParams{UpdatedAtMs: timeToMs(now), ID: id}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertMessage(q *messagedb.Queries, record storage.Message) error {
	blocks, err := marshalBlocks(record.Blocks)
	if err != nil {
		return err
	}
	return q.InsertMessage(context.Background(), messagedb.InsertMessageParams{
		ThreadID:    record.ThreadID,
		Seq:         record.Seq,
		Role:        record.Role,
		Content:     record.Content,
		Reasoning:   nullDBString(record.Reasoning),
		Blocks:      blocks,
		CreatedAtMs: timeToMs(record.CreatedAt),
	})
}

func messageFromDB(row messagedb.Message) (storage.Message, error) {
	blocks, err := unmarshalBlocks(row.Blocks)
	if err != nil {
		return storage.Message{}, err
	}
	return storage.Message{
		ThreadID:  row.ThreadID,
		Seq:       row.Seq,
		Role:      row.Role,
		Content:   row.Content,
		Reasoning: row.Reasoning.String,
		Blocks:    blocks,
		CreatedAt: msToTime(row.CreatedAtMs),
	}, nil
}
