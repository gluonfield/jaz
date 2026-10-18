package bots

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// AppVisible reports whether Jaz shows the apps a thread presents: a bot's own
// chat does, while a turn answering another bot and a bot's group threads keep
// them private.
func (s *Service) AppVisible(threadID string) bool {
	session, err := s.store.LoadSession(threadID)
	return err == nil && session.SourceType != storage.SourceBotMember && (session.Turn == nil || session.Turn.Output == nil || session.Turn.Output.ReplyTo == "")
}

// Message sends text from a thread to a bot or a group, named by id, mention
// target or name. A bot answers in a turn of its own and the answer returns to
// the sender as a new turn; a group gets the text as a post and its members
// take their turns. Bots writing to each other without the user stop after
// maxHops messages.
func (s *Service) Message(fromThread, ref, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	to, err := s.resolve(ref)
	if err != nil {
		return err
	}
	target, session, err := s.load(to)
	if err != nil {
		return err
	}
	hops := 1
	if caller, err := s.store.LoadSession(fromThread); err == nil && caller.Turn != nil && caller.Turn.Output != nil {
		hops += caller.Turn.Output.Hops
	}
	if hops > maxHops {
		return fmt.Errorf("not sent: bots have written to each other %d times since the user last wrote, which is the limit; finish with what you have", maxHops)
	}
	from, sender := s.author(fromThread)
	if target.Kind == KindGroup {
		return s.post(target, sessionevents.RoomMessageEvent{Speaker: "bot", BotID: from, Name: sender, Text: text}, hops > 1)
	}
	if to == from {
		return errors.New("a bot cannot message itself")
	}
	if err := s.queue.QueueInternalTurn(context.Background(), to, storage.QueuedMessage{
		Text: messagePrompt(sender, text), Output: &storage.TurnOutput{ReplyTo: fromThread, Hops: hops},
	}); err != nil {
		return err
	}
	s.announce(fromThread, sessionevents.BotActivityEvent{Kind: "message_sent", Label: session.Title})
	return nil
}

// Say posts a bot's message: in a turn answering another bot, to that bot;
// from the bot's thread in a group, into that group; otherwise into its own
// chat.
func (s *Service) Say(threadID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	session, err := s.store.LoadSession(threadID)
	if err != nil {
		return err
	}
	if session.Turn != nil && session.Turn.Output != nil && session.Turn.Output.ReplyTo != "" {
		return s.store.AppendTurnReply(threadID, text)
	}
	membership, err := s.store.LoadMembershipByThread(threadID)
	inGroup := err == nil
	bot := threadID
	if inGroup {
		bot = membership.BotID
	}
	record, botSession, err := s.load(bot)
	if err != nil || record.Kind != KindBot {
		return errors.New("only a Jaz bot can send messages")
	}
	message := sessionevents.RoomMessageEvent{Speaker: "bot", BotID: bot, Name: botSession.Title, Text: text}
	if !inGroup {
		return appendEvent(s.store, s.events, sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()})
	}
	group, _, err := s.load(membership.GroupID)
	if err != nil {
		return err
	}
	return s.post(group, message, false)
}

// announce notes in a thread what woke its bot; the note is best-effort.
func (s *Service) announce(threadID string, activity sessionevents.BotActivityEvent) {
	if threadID == "" {
		return
	}
	if err := Announce(s.store, s.events, threadID, activity); err != nil {
		s.log.Warn("append bot activity failed", "thread", threadID, "error", err)
	}
}

// Announce notes in a bot's thread what woke it.
func Announce(store EventStore, events Publisher, threadID string, activity sessionevents.BotActivityEvent) error {
	return appendEvent(store, events, sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeBotActivity, BotActivity: &activity, At: time.Now().UTC()})
}

// appendEvent stores event and streams the stored copy, whose seq lets
// clients match it to the same event in history.
func appendEvent(store EventStore, bus Publisher, event sessionevents.Event) error {
	events := []sessionevents.Event{event}
	if err := store.AppendSessionEvents(event.SessionID, events...); err != nil {
		return err
	}
	bus.Publish(events[0])
	return nil
}

// resolve finds a bot or group by id, mention target or exact name.
func (s *Service) resolve(ref string) (string, error) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "bot:")
	if _, _, err := s.load(ref); err == nil {
		return ref, nil
	}
	sessions, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBot})
	if err != nil {
		return "", err
	}
	for _, session := range sessions {
		if strings.EqualFold(session.Title, ref) {
			return session.ID, nil
		}
	}
	return "", fmt.Errorf("no bot or group named %q", ref)
}

// author is who a thread speaks as: the bot it belongs to, or else Jaz.
func (s *Service) author(threadID string) (string, string) {
	if bot := s.BotOf(threadID); bot != "" {
		if _, session, err := s.load(bot); err == nil {
			return bot, session.Title
		}
	}
	return threadID, "Jaz"
}

// name is how a thread signs its messages.
func (s *Service) name(threadID string) string {
	_, name := s.author(threadID)
	return name
}
