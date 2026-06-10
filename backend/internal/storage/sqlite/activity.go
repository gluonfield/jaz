package sqlite

import "github.com/wins/jaz/backend/internal/storage"

func (s *Store) LoadActivity(id string) ([]storage.ActivityEntry, error) {
	if s.mirror == nil {
		return nil, nil
	}
	return s.mirror.LoadActivity(id)
}

func (s *Store) SaveActivity(id string, activity []storage.ActivityEntry) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.SaveActivity(id, activity)
}

func (s *Store) UpsertActivity(id string, entry storage.ActivityEntry) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.UpsertActivity(id, entry)
}
