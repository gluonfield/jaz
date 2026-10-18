package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
)

func (s *Store) SaveBot(record storage.BotRecord) error {
	members, err := json.Marshal(record.Members)
	if err != nil {
		return err
	}
	if record.Members == nil {
		members = []byte("[]")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return botdb.New(s.db).UpsertBot(context.Background(), botdb.UpsertBotParams{
		ThreadID: record.ThreadID,
		Kind:     record.Kind,
		Prompt:   record.Prompt,
		Shape:    record.Shape,
		Color:    record.Color,
		Members:  string(members),
		Main:     boolInt(record.Main),
	})
}

func (s *Store) LoadBot(threadID string) (storage.BotRecord, error) {
	row, err := botdb.New(s.db).GetBot(context.Background(), threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.BotRecord{}, storage.ErrBotNotFound
	}
	if err != nil {
		return storage.BotRecord{}, err
	}
	return botFromDB(row), nil
}

func (s *Store) ListBots() ([]storage.BotRecord, error) {
	rows, err := botdb.New(s.db).ListBots(context.Background())
	if err != nil {
		return nil, err
	}
	records := make([]storage.BotRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, botFromDB(row))
	}
	return records, nil
}

// PinBots pins exactly ids, in that order, and unpins every other bot.
func (s *Store) PinBots(ids []string) error {
	ctx := context.Background()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := botdb.New(tx)
	if err := q.UnpinBots(ctx); err != nil {
		return err
	}
	for i, id := range ids {
		if err := q.PinBot(ctx, botdb.PinBotParams{Pinned: int64(i + 1), ThreadID: id}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func botFromDB(row botdb.Bot) storage.BotRecord {
	var members []string
	_ = json.Unmarshal([]byte(row.Members), &members)
	return storage.BotRecord{
		ThreadID: row.ThreadID, Kind: row.Kind, Prompt: row.Prompt, Shape: row.Shape, Color: row.Color, Pinned: int(row.Pinned), Members: members, Main: row.Main != 0,
	}
}

func (s *Store) SaveMembership(membership storage.BotMembership) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return botdb.New(s.db).SaveMembership(context.Background(), botdb.SaveMembershipParams{
		GroupID: membership.GroupID, BotID: membership.BotID, ThreadID: membership.ThreadID, Seen: membership.Seen,
	})
}

func (s *Store) LoadMembership(groupID, botID string) (storage.BotMembership, error) {
	row, err := botdb.New(s.db).GetMembership(context.Background(), botdb.GetMembershipParams{GroupID: groupID, BotID: botID})
	return membershipFromDB(row, err)
}

func (s *Store) LoadMembershipByThread(threadID string) (storage.BotMembership, error) {
	row, err := botdb.New(s.db).GetMembershipByThread(context.Background(), threadID)
	return membershipFromDB(row, err)
}

func (s *Store) ListMemberships() ([]storage.BotMembership, error) {
	rows, err := botdb.New(s.db).ListMemberships(context.Background())
	if err != nil {
		return nil, err
	}
	memberships := make([]storage.BotMembership, 0, len(rows))
	for _, row := range rows {
		membership, _ := membershipFromDB(row, nil)
		memberships = append(memberships, membership)
	}
	return memberships, nil
}

func membershipFromDB(row botdb.BotMembership, err error) (storage.BotMembership, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return storage.BotMembership{}, storage.ErrMembershipNotFound
	}
	if err != nil {
		return storage.BotMembership{}, err
	}
	return storage.BotMembership{GroupID: row.GroupID, BotID: row.BotID, ThreadID: row.ThreadID, Seen: row.Seen}, nil
}
