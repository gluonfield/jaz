package bots

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	// maxFollowUps bounds how often members' posts reach each other between two
	// posts from the user, as a new turn or mid-turn, so bots answering each
	// other cannot run on.
	maxFollowUps = 12
	maxHistory   = 20
	// steerTimeout bounds how long handing messages to a running turn waits.
	steerTimeout = time.Minute
)

// Post adds the user's message to a group, with the files uploaded to the
// group's thread that attachmentIDs name.
func (s *Service) Post(groupID, text string, attachmentIDs []string) error {
	record, _, err := s.load(groupID)
	if err != nil {
		return err
	}
	if record.Kind != KindGroup {
		return errors.New("not a group")
	}
	attachments, err := s.attachments.ResolveAttachments(record.ThreadID, attachmentIDs)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return errors.New("message is required")
	}
	return s.post(record, sessionevents.RoomMessageEvent{Speaker: "user", Name: "You", Text: text, Attachments: attachments})
}

// post records a message in a group and hands it to every other member, which
// decides for itself whether to answer.
func (s *Service) post(group storage.BotRecord, message sessionevents.RoomMessageEvent) error {
	message.Text = s.linkMentions(message.Text, group.Members)
	if err := s.appendEvent(sessionevents.Event{SessionID: group.ThreadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()}); err != nil {
		return err
	}
	fromMember := slices.Contains(group.Members, message.BotID)
	if !fromMember {
		s.mu.Lock()
		s.followUps[group.ThreadID] = 0
		s.mu.Unlock()
	}
	for _, member := range group.Members {
		if member != message.BotID {
			go s.deliver(group, member, fromMember)
		}
	}
	return nil
}

// deliver shows member the group messages it has not seen, one delivery at a
// time per member so none is shown twice, and tells the group when member
// cannot be reached.
func (s *Service) deliver(group storage.BotRecord, member string, fromMember bool) {
	lock := s.deliveryLock(group.ThreadID, member)
	lock.Lock()
	defer lock.Unlock()
	if err := s.deliverLocked(group, member, fromMember); err != nil {
		s.log.Warn("group delivery failed", "group", group.ThreadID, "member", member, "error", err)
		s.announce(group.ThreadID, sessionevents.BotActivityEvent{Kind: "unreachable", Label: s.name(member) + " · " + err.Error()})
	}
}

// deliverLocked hands the messages to the turn member's thread for the group
// is taking or, when it takes none or that turn cannot take them, queues a
// turn for them on that thread, and marks them seen. Deliveries prompted by a
// member's post spend the group's follow-ups.
func (s *Service) deliverLocked(group storage.BotRecord, member string, fromMember bool) error {
	membership, err := s.store.LoadMembership(group.ThreadID, member)
	if errors.Is(err, storage.ErrMembershipNotFound) {
		membership, err = s.join(group.ThreadID, member)
	}
	if err != nil {
		return err
	}
	messages, seen, err := s.unseen(group.ThreadID, member, membership.Seen)
	if err != nil || len(messages) == 0 {
		return err
	}
	thread, err := s.store.LoadSession(membership.ThreadID)
	if err != nil {
		return err
	}
	if fromMember && !s.spendFollowUp(group.ThreadID) {
		return nil
	}
	if thread.Turn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), steerTimeout)
		_, err := s.threads.SteerInternal(ctx, membership.ThreadID, groupUpdatePrompt(messages))
		cancel()
		if err == nil {
			membership.Seen = seen
			return s.store.SaveMembership(membership)
		}
		s.log.Debug("group turn could not take messages; queueing a turn", "group", group.ThreadID, "member", member, "error", err)
	}
	peers := make([]string, 0, len(group.Members))
	for _, other := range group.Members {
		if other != member {
			peers = append(peers, fmt.Sprintf("[@%s](bot:%s)", s.name(other), other))
		}
	}
	if err := s.queue.QueueInternalTurn(context.Background(), membership.ThreadID, storage.NewInternalQueuedMessage(groupTurnPrompt(peers, messages))); err != nil {
		return err
	}
	membership.Seen = seen
	return s.store.SaveMembership(membership)
}

// deliveryLock serialises deliveries to member in the group.
func (s *Service) deliveryLock(groupID, member string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := groupID + "\x00" + member
	lock := s.delivering[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.delivering[key] = lock
	}
	return lock
}

// spendFollowUp takes one of the group's follow-ups, reporting false once
// they are spent.
func (s *Service) spendFollowUp(groupID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.followUps[groupID] >= maxFollowUps {
		return false
	}
	s.followUps[groupID]++
	return true
}

// join creates the thread member takes the group's turns in: a hidden thread
// that the agent manager runs like the bot itself.
func (s *Service) join(groupID, member string) (storage.BotMembership, error) {
	bot, err := s.store.LoadSession(member)
	if err != nil {
		return storage.BotMembership{}, err
	}
	group, err := s.store.LoadSession(groupID)
	if err != nil {
		return storage.BotMembership{}, err
	}
	thread, err := s.threads.CreateSession(context.Background(), acp.SpawnRequest{
		Slug:       bot.Title + " in " + group.Title,
		Title:      bot.Title + " in " + group.Title,
		SourceType: storage.SourceBotMember,
		SourceID:   member,
	})
	if err != nil {
		return storage.BotMembership{}, err
	}
	membership := storage.BotMembership{GroupID: groupID, BotID: member, ThreadID: thread.ID}
	return membership, s.store.SaveMembership(membership)
}

// unseen returns the group messages after seq after that member did not post,
// with the seq that marks them seen. With after zero, as before member has
// been shown anything in its group thread, it returns those after member's own
// last post.
func (s *Service) unseen(groupID, member string, after int64) ([]sessionevents.RoomMessageEvent, int64, error) {
	events, err := s.store.LoadSessionEvents(groupID)
	if err != nil {
		return nil, 0, err
	}
	seen := after
	var messages []sessionevents.RoomMessageEvent
	for _, event := range events {
		message := event.RoomMessage
		if message == nil || event.Seq <= after {
			continue
		}
		seen = event.Seq
		switch {
		case message.BotID != member:
			messages = append(messages, *message)
		case after == 0:
			messages = messages[:0]
		}
	}
	if len(messages) > maxHistory {
		messages = messages[len(messages)-maxHistory:]
	}
	return messages, seen, nil
}
