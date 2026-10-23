package bots

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

// NoteStore is what the note a bot's turn opens with is read from.
type NoteStore interface {
	PromptStore
	ListOpenCommitments(botID string) ([]storage.Commitment, error)
}

// shown is what a bot thread's agent was shown when its turn started, which
// the thread keeps as its PromptDigest once the turn succeeds: the version of
// the bot's identity, and the digest of each open task's line with the session
// and context size it was shown in. A digest from before tasks were kept is
// the identity's version alone.
type shown struct {
	Identity string           `json:"identity,omitempty"`
	Session  string           `json:"session,omitempty"`
	Context  int64            `json:"context,omitempty"`
	Tasks    map[int64]string `json:"tasks,omitempty"`
}

func lastShown(digest string) shown {
	var last shown
	if json.Unmarshal([]byte(digest), &last) != nil {
		return shown{Identity: digest}
	}
	return last
}

// held is the digest of each task line the agent still holds: none once its
// session changed or its context shrank, as a compaction does. Lines shown
// before the session had an id were shown in the turn that started it.
func (s shown) held(thread storage.Session) map[int64]string {
	if s.Session != "" && s.Session != thread.RuntimeRef.SessionID || thread.Usage.ContextTokens < s.Context {
		return nil
	}
	return s.Tasks
}

// TurnNote opens a turn in a bot's own thread with its open tasks, and there
// or in one of its group threads with what changed since its agent was last
// shown it: the bot's identity, and the identity files that changed since its
// last successful turn started. Agents load those files as a session starts,
// not every turn.
func TurnNote(store NoteStore) func(storage.Session) (note, promptDigest string) {
	return func(thread storage.Session) (string, string) {
		if thread.SourceType != storage.SourceBot && thread.SourceType != storage.SourceBotMember || thread.RuntimeRef == nil {
			return "", ""
		}
		last := lastShown(thread.PromptDigest)
		note, identity := identityNote(store, thread, last.Identity)
		note += changedFilesNote(thread)
		now := shown{Identity: identity, Session: thread.RuntimeRef.SessionID, Context: thread.Usage.ContextTokens}
		if thread.SourceType == storage.SourceBot {
			var tasks string
			tasks, now.Tasks = taskNote(store, thread.ID, last.held(thread))
			note += tasks
		}
		recorded, _ := json.Marshal(now)
		return note, string(recorded)
	}
}

// SteerNote opens a message steering a turn in a bot's own thread with the
// tasks that changed since the turn started.
func SteerNote(store NoteStore) func(storage.Session) string {
	return func(thread storage.Session) string {
		if thread.SourceType != storage.SourceBot || thread.RuntimeRef == nil {
			return ""
		}
		last := thread.PromptDigest
		if thread.Turn != nil {
			last = cmp.Or(thread.Turn.PromptDigest, last)
		}
		note, _ := taskNote(store, thread.ID, lastShown(last).held(thread))
		return note
	}
}

// taskNote lists a bot's open tasks whose line its agent does not hold, by the
// digests in held, with the numbers of the rest; lines are the digests of the
// current lines.
func taskNote(store NoteStore, bot string, held map[int64]string) (note string, lines map[int64]string) {
	open, err := store.ListOpenCommitments(bot)
	if err != nil {
		return fmt.Sprintf("Your open tasks could not be read: %v.\n\n", err), nil
	}
	lines = make(map[int64]string, len(open))
	var changed strings.Builder
	var unchanged []string
	for _, commitment := range open {
		line, err := taskLine(store, commitment)
		if err != nil {
			return fmt.Sprintf("Your open tasks could not be read: %v.\n\n", err), nil
		}
		lines[commitment.ID] = digest([]byte(line))
		if held[commitment.ID] == lines[commitment.ID] {
			unchanged = append(unchanged, fmt.Sprintf("#%d", commitment.ID))
			continue
		}
		changed.WriteString(line + "\n")
	}
	if changed.Len() == 0 {
		return "", lines
	}
	note = "Your open tasks that are new or changed since you last saw them. Deliver completed results with bot_task_complete using their ids; cancel what is no longer wanted with bot_task_cancel:\n" + changed.String()
	if len(unchanged) > 0 {
		note += "Unchanged: " + strings.Join(unchanged, ", ") + ".\n"
	}
	return note + "\n", lines
}

func taskLine(store PromptStore, commitment storage.Commitment) (string, error) {
	_, name, err := owedTo(store, commitment)
	if err != nil {
		return "", err
	}
	name = cmp.Or(name, "the user")
	var b strings.Builder
	fmt.Fprintf(&b, "#%d for %s: %s", commitment.ID, name, commitment.Request)
	for _, thread := range commitment.Threads {
		fmt.Fprintf(&b, " · thread %s %s", thread.Slug, thread.State)
	}
	if commitment.Note != "" {
		fmt.Fprintf(&b, " · next: %s", commitment.Note)
	}
	b.WriteString(standing(commitment, name))
	return b.String(), nil
}
