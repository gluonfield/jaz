package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/loopdb"
)

// LastUserMessageAt is when the user last wrote to Jaz: a message in a chat or
// bot chat of their own, a group post or a voice message.
func (s *Store) LastUserMessageAt() (time.Time, error) {
	ctx := context.Background()
	q := loopdb.New(s.db)
	written, err := q.GetLastUserMessageAt(ctx)
	if err != nil {
		return time.Time{}, err
	}
	spoken, err := q.GetLastUserSpeechAt(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, err
	}
	return msToTime(max(written, spoken)), nil
}

func (s *Store) AwaySince() (time.Time, error) {
	setting, err := s.LoadSetting("loops", "away_since")
	if errors.Is(err, storage.ErrSettingNotFound) {
		return time.Time{}, nil
	}
	var since time.Time
	if err == nil {
		err = json.Unmarshal(setting.Value, &since)
	}
	return since, err
}

func (s *Store) SetAwaySince(since time.Time) error {
	if since.IsZero() {
		err := s.DeleteSetting("loops", "away_since")
		if errors.Is(err, storage.ErrSettingNotFound) {
			return nil
		}
		return err
	}
	raw, err := json.Marshal(since)
	if err == nil {
		_, err = s.SaveSetting("loops", "away_since", raw)
	}
	return err
}
