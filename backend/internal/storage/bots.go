package storage

import "errors"

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

var ErrMembershipNotFound = errors.New("bot membership not found")

// BotMembership is a bot's place in a group: the thread it takes its group
// turns in, and the seq of the last group message shown to it there.
type BotMembership struct {
	GroupID  string
	BotID    string
	ThreadID string
	Seen     int64
}
