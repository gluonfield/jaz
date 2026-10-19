package bots

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

// wakePrompt wakes a bot once about what it owes that stopped moving.
const wakePrompt = "[owed] Your turn ended while nothing moves on some of what you owe: no thread works on it and it waits on nobody. The list above marks it. Carry each on now, deliver it with done, set waiting if it waits on its requester or an outside job, or drop it. Jaz reminds you once; if one still does not move, the user is told it stalled."

// TurnEnded follows a thread's turn once Jaz has recorded how it ended: it
// notes changes to its bot's identity files and, after a turn of a bot's own
// thread or of a thread it started, looks at what the bot owes. A commitment
// that nothing moves and nothing waits on, while the bot has nothing left to
// do, wakes the bot once; if it still has not moved when the bot's next turn
// ends, the user is told it stalled. Nothing runs on a timer, so nothing runs
// while work moves.
func (s *Service) TurnEnded(threadID string) {
	s.NoteHomeChanges(threadID)
	session, err := s.store.LoadSession(threadID)
	if err != nil {
		return
	}
	switch session.SourceType {
	case storage.SourceBot:
		s.watch(threadID)
	case storage.SourceBotWorker:
		s.watch(session.SourceID)
	}
}

// WatchAll looks at what every bot owes, as Jaz starts once recovery has
// resumed the turns and queues the last process left.
func (s *Service) WatchAll() error {
	records, err := s.store.ListBots()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Kind == KindBot {
			s.watch(record.ThreadID)
		}
	}
	return nil
}

// watch wakes bot about what stalls, then tells the user about what still
// stalls after it was woken. The storage writes decide both, so a restart or
// a second look at the same moment wakes and tells only once.
func (s *Service) watch(bot string) {
	err := s.queue.QueueInternalTurn(context.Background(), storage.TurnDelivery{SessionID: bot, Message: storage.NewInternalQueuedMessage(wakePrompt), Wake: true})
	if err != nil && !errors.Is(err, storage.ErrNothingStalled) {
		s.log.Warn("waking a bot about what it owes failed", "bot", bot, "error", err)
	}
	stalled, err := s.store.MarkStalled(bot)
	if err != nil {
		s.log.Warn("marking what a bot owes as stalled failed", "bot", bot, "error", err)
		return
	}
	for _, commitment := range stalled {
		_, name, err := owedTo(s.store, commitment)
		if err == nil {
			err = s.Notice(bot, fmt.Sprintf("Stalled: “%s”, owed to %s. Nothing has moved on it since I was reminded. Tell me how to go on, or to drop it.", commitment.Request, cmp.Or(name, "you")))
		}
		if err != nil {
			s.log.Warn("telling the user a commitment stalled failed", "bot", bot, "commitment", commitment.ID, "error", err)
		}
	}
}
