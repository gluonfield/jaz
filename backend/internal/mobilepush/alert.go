package mobilepush

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Alert is what a person is told about a conversation: a task done, a reply,
// a question for them, or work stopped. iPhones and desktops get the same
// alerts.
type Alert struct {
	SessionID string        `json:"session_id"`
	Key       string        `json:"key"`
	State     string        `json:"state"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name,omitempty"`
	Avatars   []bots.Avatar `json:"avatars,omitempty"`
	Title     string        `json:"title,omitempty"`
	Body      string        `json:"body"`
	RequestID string        `json:"request_id,omitempty"`
	At        time.Time     `json:"at"`
	// secret marks a question whose text is withheld.
	secret bool
}

// classify decides what an event tells the person from typed turn and task
// state, never from what a message says.
func (s *Service) classify(session storage.Session, event sessionevents.Event) (*Alert, error) {
	switch event.Type {
	case sessionevents.TypeRoomMessage:
		return s.taskChanged(session, event)
	case "permission_request":
		return s.question(session, event)
	case sessionevents.TypeTurnFinished:
		return s.turnEnded(session, event)
	}
	return nil, nil
}

// taskChanged tells of a bot's message that completed, asked about or stopped
// a task. Every other bot message, acknowledgements and progress included, is
// silent.
func (s *Service) taskChanged(session storage.Session, event sessionevents.Event) (*Alert, error) {
	message := event.RoomMessage
	if message.Task == nil || len(message.Task.IDs) == 0 {
		return nil, nil
	}
	alert, visible, err := s.from(session)
	if err != nil || !visible {
		return nil, err
	}
	alert.Body = outcome(message.Text)
	switch message.Task.Status {
	case storage.CommitmentDone:
		alert.State = "done"
	case storage.CommitmentWaitingForUser:
		alert.State = "needs_you"
	case storage.CommitmentStopped:
		alert.State = "stopped"
		alert.Body = firstLine(message.Text)
	default:
		return nil, nil
	}
	task, err := s.store.LoadCommitment(message.Task.IDs[0])
	if err != nil {
		return nil, err
	}
	alert.Title = bots.Preview(task.Request)
	alert.Key = fmt.Sprintf("%s:%d", event.Type, event.Seq)
	alert.At = event.At
	return &alert, nil
}

// question asks the person in the thread holding it: a plain thread, a thread
// started under one, or a bot's chat. Only questions and plan decisions reach
// this; tool permissions are granted as they arrive.
func (s *Service) question(session storage.Session, event sessionevents.Event) (*Alert, error) {
	permission := event.Permission
	if permission == nil || permission.SessionID != session.ID {
		return nil, nil
	}
	alert, visible, err := s.from(session)
	if err == nil && !visible {
		visible, err = s.underPlainThread(session)
	}
	if err != nil || !visible {
		return nil, err
	}
	alert.State = "needs_you"
	alert.Key = "question:" + permission.ID
	alert.RequestID = permission.ID
	alert.At = event.At
	alert.Body = "Your answer is needed"
	alert.secret = slices.ContainsFunc(permission.Questions, func(question sessionevents.ACPQuestion) bool { return question.IsSecret })
	if open := slices.IndexFunc(permission.Questions, func(question sessionevents.ACPQuestion) bool { return !question.IsSecret }); open >= 0 {
		alert.Body = bots.Preview(permission.Questions[open].Question)
	}
	return &alert, nil
}

// turnEnded tells of a turn that failed, or of the reply a turn left: a plain
// thread's final message or a bot's new reply. A member's turn tells its
// group. Cancels and restarts are silent, as is a compaction, which leaves no
// reply.
func (s *Service) turnEnded(session storage.Session, event sessionevents.Event) (*Alert, error) {
	finished := event.ACP
	if finished == nil {
		return nil, nil
	}
	room, bot, err := s.room(session)
	if err != nil {
		return nil, err
	}
	alert, visible, err := s.from(room)
	if err != nil || !visible || room.Archived {
		return nil, err
	}
	alert.At = event.At
	switch {
	case finished.State == "failed":
		alert.State = "stopped"
		alert.Key = "failed:" + event.At.UTC().Format(time.RFC3339Nano)
		alert.Body = cmp.Or(firstLine(finished.Error), "Work stopped with an error.")
		return &alert, nil
	case finished.State != "idle" || finished.StopReason != "end_turn":
		return nil, nil
	case alert.Kind != "chat":
		return s.newReply(alert, room.ID, bot, finished.TurnStartedAt)
	case finished.Assistant == "" || session.CompletedAt.IsZero():
		return nil, nil
	}
	alert.State = "reply"
	alert.Key = "completed:" + session.CompletedAt.UTC().Format(time.RFC3339Nano)
	alert.Body = outcome(finished.Assistant)
	return &alert, nil
}

// newReply is a bot's final message in a turn a person started that opened
// no task: the newest message in its chat, sent by the bot in that turn and
// changing no task, which told already.
func (s *Service) newReply(alert Alert, room, bot string, turn time.Time) (*Alert, error) {
	if turn.IsZero() {
		return nil, nil
	}
	latest, ok, err := s.store.LoadLatestSessionEvent(room, sessionevents.TypeRoomMessage)
	if err != nil || !ok {
		return nil, err
	}
	message := latest.RoomMessage
	if message == nil || message.BotID != bot || message.Task != nil || !message.Answers.Equal(turn) {
		return nil, nil
	}
	opened, err := s.store.OpenedTask(bot, turn)
	if err != nil || opened {
		return nil, err
	}
	alert.State = "reply"
	alert.Key = fmt.Sprintf("%s:%d", latest.Type, latest.Seq)
	alert.Body = outcome(message.Text)
	return &alert, nil
}

// from is who an alert about a thread comes from: the thread itself, or the
// bot or group whose chat it is. Only top-level plain threads and bot and
// group chats tell on their own.
func (s *Service) from(session storage.Session) (Alert, bool, error) {
	if session.SourceType != storage.SourceBot {
		alert := Alert{SessionID: session.ID, Kind: "chat", Title: bots.Preview(cmp.Or(session.Title, session.Slug))}
		return alert, session.SourceType == "" && session.ParentID == "", nil
	}
	record, err := s.store.LoadBot(session.ID)
	if err != nil {
		return Alert{}, false, err
	}
	faces := []string{session.ID}
	if record.Kind == bots.KindGroup {
		faces = record.Members[:min(2, len(record.Members))]
	}
	alert := Alert{SessionID: session.ID, Kind: record.Kind, Name: session.Title}
	for _, id := range faces {
		face, err := s.store.LoadBot(id)
		if err != nil {
			return Alert{}, false, err
		}
		alert.Avatars = append(alert.Avatars, bots.Avatar{Shape: face.Shape, Color: face.Color})
	}
	return alert, true, nil
}

// room is the chat a thread's turns post to and the bot posting there: a
// member's thread posts into its group as that member, any other thread into
// itself.
func (s *Service) room(session storage.Session) (storage.Session, string, error) {
	if session.SourceType != storage.SourceBotMember {
		return session, session.ID, nil
	}
	membership, err := s.store.LoadMembershipByThread(session.ID)
	if err != nil {
		return storage.Session{}, "", err
	}
	group, err := s.store.LoadSession(membership.GroupID)
	return group, membership.BotID, err
}

// underPlainThread reports whether a thread was started, however deep, under
// a plain top-level thread.
func (s *Service) underPlainThread(session storage.Session) (bool, error) {
	for session.ParentID != "" {
		parent, err := s.store.LoadSession(session.ParentID)
		if err != nil {
			return false, err
		}
		session = parent
	}
	return session.SourceType == "", nil
}

// outcome is a message's first sentence as plain text, cut like a bot
// preview. Headings and code are not prose and are left out.
func outcome(message string) string {
	source := []byte(message)
	var plain strings.Builder
	_ = ast.Walk(goldmark.DefaultParser().Parse(text.NewReader(source)), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch node := node.(type) {
		case *ast.Heading, *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			if entering {
				plain.Write(node.Segment.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					plain.WriteByte(' ')
				}
			}
		}
		if !entering && node.Type() == ast.TypeBlock {
			plain.WriteByte('\n')
		}
		return ast.WalkContinue, nil
	})
	lead, _, _ := strings.Cut(strings.TrimSpace(plain.String()), "\n")
	for i, r := range lead {
		if strings.ContainsRune(".!?", r) && (i+1 == len(lead) || lead[i+1] == ' ') {
			return bots.Preview(lead[:i+1])
		}
	}
	return bots.Preview(lead)
}

// firstLine is a text's first line, cut like a bot preview: an error without
// any trace that follows it.
func firstLine(value string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(value), "\n")
	return bots.Preview(line)
}
