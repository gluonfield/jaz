package bots

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/loops"
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
// take their turns. A chain of direct messages between bots, each sent from a
// turn the one before it led to, stops after maxHops without the user. A
// message with done closes those commitments and goes to the thread of the
// bot that asked for them.
func (s *Service) Message(fromThread, ref, text string, done ...int64) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	caller, err := s.store.LoadSession(fromThread)
	if err == nil && caller.SourceType == storage.SourceBotWorker {
		return errors.New("a thread a bot started cannot message bots; put what you need from them in your report")
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
	if caller.Turn != nil && caller.Turn.Output != nil {
		hops += caller.Turn.Output.Hops
	}
	if hops > maxHops {
		return fmt.Errorf("not sent: bots have written to each other %d times since the user last wrote, which is the limit; finish with what you have", maxHops)
	}
	from, sender := author(s.store, fromThread)
	// Commitments are delivered where they were asked; the first decides
	// where, and the rest must have been asked there too.
	thread := to
	if len(done) > 0 {
		asked, _, err := s.owed(fromThread, done[0])
		if err != nil {
			return err
		}
		if asked != "" && botOf(s.store, asked) == to {
			thread = asked
		}
	}
	if err := s.delivers(fromThread, thread, done); err != nil {
		return err
	}
	if target.Kind == KindGroup {
		return s.post(target, sessionevents.RoomMessageEvent{Speaker: "bot", BotID: from, Name: sender, Text: text}, hops > 1)
	}
	if to == from {
		return errors.New("a bot cannot message itself")
	}
	if err := s.queue.QueueInternalTurn(context.Background(), storage.TurnDelivery{SessionID: thread, Closes: done, Message: storage.QueuedMessage{
		Text: messagePrompt(sender, text), Output: &storage.TurnOutput{ReplyTo: fromThread, Hops: hops},
	}}); err != nil {
		return explain(err)
	}
	s.exchanged(from, sender, to, session.Title, text)
	return nil
}

// exchanged records a message between bots in the threads of both, whichever
// of their threads it was sent from.
func (s *Service) exchanged(from, sender, to, recipient, text string) {
	now := time.Now()
	s.record(from, sessionevents.BotActivityEvent{Kind: "message_sent", Label: recipient, Text: text}, now)
	s.record(to, sessionevents.BotActivityEvent{Kind: "message_received", Label: sender, Text: text}, now)
}

// Say posts a bot's message: in a turn answering another bot, to that bot;
// from the bot's thread in a group, into that group; otherwise into its own
// chat. The commitments in done close as the message reaches whoever they
// are owed to.
func (s *Service) Say(threadID, text string, done ...int64) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	session, err := s.store.LoadSession(threadID)
	if err != nil {
		return err
	}
	var output *storage.TurnOutput
	to := ""
	if session.Turn != nil && session.Turn.Output != nil && session.Turn.Output.ReplyTo != "" {
		output = session.Turn.Output
		if to, err = recipient(s.store, output.ReplyTo); err != nil {
			return err
		}
	}
	if err := s.delivers(threadID, to, done); err != nil {
		return err
	}
	if output != nil {
		if err := s.store.AppendTurnReply(threadID, text, done); err != nil {
			return explain(err)
		}
		// A routine's thread reports to its bot; any other reply answers a bot.
		if output.ReplyTo != session.Owner() {
			from, sender := author(s.store, threadID)
			to, name := author(s.store, output.ReplyTo)
			s.exchanged(from, sender, to, name, text)
		}
		return nil
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
		stored, err := s.store.DeliverMessage(sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()}, done)
		if err != nil {
			return explain(err)
		}
		s.events.Publish(stored)
		return nil
	}
	group, _, err := s.load(membership.GroupID)
	if err != nil {
		return err
	}
	return s.post(group, message, false)
}

// Answer is the delivery of the answer a finished turn gave the bot it
// answers, if any: a routine run's report when a thread answers the bot it
// works for, or else a reply, which a bot's own chat takes back for its user
// when the bot it answers is gone.
func Answer(session storage.Session) *storage.TurnDelivery {
	output := session.Turn.Output
	if output == nil || output.ReplyTo == "" || len(output.Replies) == 0 {
		return nil
	}
	answer := strings.Join(output.Replies, "\n\n")
	text := replyPrompt(session.Title, answer)
	if output.ReplyTo == session.Owner() {
		text = loops.Report(session.Title, answer)
	}
	// The answer continues the exchange between bots that led to it.
	message := storage.NewInternalQueuedMessage(text)
	message.Output = &storage.TurnOutput{Hops: output.Hops}
	delivery := &storage.TurnDelivery{SessionID: output.ReplyTo, Message: message, Closes: output.Done}
	if session.SourceType == storage.SourceBot {
		undelivered := storage.NewInternalQueuedMessage("[undelivered] The bot you answered is gone, so your answer never reached it:\n\n" + answer + "\n\nPass on to the user, with send_message, what of it matters to them.")
		delivery.Fallback = &undelivered
	}
	return delivery
}

// Notice posts a bot's message in its own chat outside any turn, as news the
// user has not read.
func (s *Service) Notice(botID, text string) error {
	_, session, err := s.load(botID)
	if err != nil {
		return err
	}
	message := sessionevents.RoomMessageEvent{Speaker: "bot", BotID: botID, Name: session.Title, Text: text}
	if err := appendEvent(s.store, s.events, sessionevents.Event{SessionID: botID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()}); err != nil {
		return err
	}
	if err := s.store.SetThreadUnread(botID, true); err != nil {
		return err
	}
	s.events.Publish(sessionevents.Event{SessionID: botID, Type: sessionevents.TypeSession})
	return nil
}

// record notes in a thread what its bot did at, best-effort.
func (s *Service) record(threadID string, activity sessionevents.BotActivityEvent, at time.Time) {
	if threadID == "" {
		return
	}
	if err := appendEvent(s.store, s.events, sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeBotActivity, BotActivity: &activity, At: at.UTC()}); err != nil {
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
func author(store PromptStore, threadID string) (string, string) {
	if bot := botOf(store, threadID); bot != "" {
		if session, err := store.LoadSession(bot); err == nil {
			return bot, session.Title
		}
	}
	return threadID, "Jaz"
}

// recipient is the thread an answer to thread reaches: thread itself while it
// is there, or else the user, as empty.
func recipient(store PromptStore, thread string) (string, error) {
	if thread == "" {
		return "", nil
	}
	session, err := store.LoadSession(thread)
	if errors.Is(err, storage.ErrSessionNotFound) || err == nil && session.Archived {
		return "", nil
	}
	return thread, err
}

// name is how a thread signs its messages.
func (s *Service) name(threadID string) string {
	_, name := author(s.store, threadID)
	return name
}
