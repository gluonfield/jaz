package bots

import (
	"context"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

const wakePrompt = "[owed] Your turn ended with unfinished tasks and no active worker or unmet prerequisite. Reconcile each task now: continue it, deliver its result with bot_task_update status completed and message, ask an actual missing question with status waiting_for_user and message, or cancel it with a reason. Use bot_task_list for the full current list."

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

func (s *Service) WatchAll() error {
	bots, err := s.store.ListWatchBots()
	if err != nil {
		return err
	}
	for _, bot := range bots {
		s.watch(bot)
	}
	return nil
}

func (s *Service) RunWatch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.WatchAll(); err != nil {
				s.log.Warn("checking unfinished bot tasks failed", "error", err)
			}
		}
	}
}

func (s *Service) watch(bot string) {
	err := s.queue.QueueInternalTurn(context.Background(), storage.TurnDelivery{SessionID: bot, Message: storage.NewInternalQueuedMessage(wakePrompt), Wake: true})
	if err != nil && !errors.Is(err, storage.ErrNothingStalled) {
		s.log.Warn("waking a bot about its unfinished tasks failed", "bot", bot, "error", err)
	}
}
