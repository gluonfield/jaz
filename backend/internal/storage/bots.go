package storage

import (
	"errors"
	"time"
)

var ErrBotNotFound = errors.New("bot not found")

// BotRecord is what a bot adds to its thread: the thread holds the name, agent
// and history; the record holds the prompt, avatar, pin position (1-based,
// 0 unpinned), group members and whether it is the user's main bot.
type BotRecord struct {
	ThreadID string
	Kind     string
	Prompt   string
	Shape    string
	Color    string
	Pinned   int
	Members  []string
	Main     bool
}

// ErrMainBot refuses archiving the user's main bot, which stays.
var ErrMainBot = errors.New("the main bot cannot be archived")

var ErrMembershipNotFound = errors.New("bot membership not found")

// BotMembership is a bot's place in a group: the thread it takes its group
// turns in, and the seq of the last group message shown to it there.
type BotMembership struct {
	GroupID  string
	BotID    string
	ThreadID string
	Seen     int64
}

const (
	CommitmentOpen    = "open"
	CommitmentDone    = "done"
	CommitmentDropped = "dropped"
)

// ErrCommitmentNotOpen refuses a commitment that is closed or another bot's.
var ErrCommitmentNotOpen = errors.New("no such open commitment")

// ErrCommitmentWorking refuses closing a commitment while one of its threads
// works.
var ErrCommitmentWorking = errors.New("one of its threads is still working; wait for its result or stop it with stop_thread")

// Commitment is something a bot owes beyond the turn it was asked in. It
// closes as done only with a delivery to its requester, or as dropped.
type Commitment struct {
	ID    int64
	BotID string
	// Requester is the thread its result goes to, where another bot asked;
	// empty for the user.
	Requester string
	Request   string
	// Note is the bot's next step while it is open, or why it was dropped.
	Note     string
	Status   string
	OpenedAt time.Time
	ClosedAt time.Time
	// Threads are the threads the bot started for it, oldest first.
	Threads []string
}
