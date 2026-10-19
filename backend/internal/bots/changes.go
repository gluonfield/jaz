package bots

import (
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// ChangedBots includes a worker's owner and the room a group member works in.
func (s *Service) ChangedBots(event sessionevents.Event) []string {
	thread, err := s.store.LoadSession(event.SessionID)
	if err != nil {
		return nil
	}
	id := thread.Owner()
	if _, err := s.store.LoadBot(id); err != nil {
		return nil
	}
	ids := []string{id}
	if thread.SourceType == storage.SourceBotMember {
		if membership, err := s.store.LoadMembershipByThread(thread.ID); err == nil {
			ids = append(ids, membership.GroupID)
		}
	}
	return ids
}
