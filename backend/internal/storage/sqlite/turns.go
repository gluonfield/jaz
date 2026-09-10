package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

func (s *Store) StartSessionTurn(id string, turn storage.Turn) error {
	s.writeMu.Lock()
	err := threaddb.New(s.db).StartSessionTurn(context.Background(), threaddb.StartSessionTurnParams{
		ID: id, Turn: sessionTurnJSON(&turn), StartedAtMs: timeToMs(time.Now().UTC()),
	})
	s.writeMu.Unlock()
	if err == nil {
		if current, loadErr := s.LoadSession(id); loadErr == nil {
			s.mirrorSession(current)
		}
	}
	return err
}

func sessionTurnJSON(turn *storage.Turn) string {
	if turn == nil {
		return ""
	}
	data, _ := json.Marshal(turn)
	return string(data)
}

func parseSessionTurn(raw string) (*storage.Turn, error) {
	var turn *storage.Turn
	if raw == "" {
		return nil, nil
	}
	err := json.Unmarshal([]byte(raw), &turn)
	return turn, err
}
