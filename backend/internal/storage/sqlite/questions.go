package sqlite

import (
	"context"
	"encoding/json"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
)

func (s *Store) HasOpenQuestion(id string) (bool, error) {
	return eventdb.New(s.db).HasOpenQuestion(context.Background(), id)
}

func (s *Store) LoadSessionQuestionEvents(id string) ([]sessionevents.Event, error) {
	rows, err := eventdb.New(s.db).ListSessionQuestionEvents(context.Background(), id)
	if err != nil {
		return nil, err
	}
	events := make([]sessionevents.Event, 0, len(rows))
	for _, row := range rows {
		event := sessionevents.Event{SessionID: id, Seq: row.Seq, Type: row.Type}
		if err := json.Unmarshal([]byte(row.Permission.String), &event.Permission); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
