package bots

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
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
// session changed or its context shrank, as a compaction does.
func (s shown) held(thread storage.Session) map[int64]string {
	if s.Session != thread.RuntimeRef.SessionID || thread.Usage.ContextTokens < s.Context {
		return nil
	}
	return s.Tasks
}

// TurnNote opens a turn in a bot's own thread with its tasks, and there or in
// one of its group threads with what changed since its agent was last shown
// it: the bot's identity, and the identity files that changed since its last
// successful turn started. Agents load those files as a session starts, not
// every turn.
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

// identityNote shows an agent that keeps the prompt its thread started with
// the bot's identity again when it changed since the version it was last
// shown, seen; version is the one the agent holds after the note. A thread
// whose agent has not started its conversation yet starts with the current
// identity.
func identityNote(store PromptStore, thread storage.Session, seen string) (note, version string) {
	if !acp.KeepsStartPrompt(thread.RuntimeRef.Agent) {
		return "", ""
	}
	identity, err := Prompt(store, thread)
	if err != nil || identity == nil {
		return "", seen
	}
	text := identity.Text()
	version = digest([]byte(text))
	if thread.RuntimeRef.SessionID == "" || seen == version {
		return "", version
	}
	return "These are your current bot instructions. They replace any earlier version in this conversation:\n\n<bot_instructions>\n" + text + "\n</bot_instructions>\n\n", version
}

func changedFilesNote(thread storage.Session) string {
	if thread.RuntimeRef.Cwd == "" {
		return ""
	}
	since := thread.CompletedTurnStartedAt
	if since.IsZero() {
		since = thread.CreatedAt
	}
	var changed []string
	for _, name := range homeFiles {
		if info, err := os.Stat(filepath.Join(thread.RuntimeRef.Cwd, name)); err == nil && info.ModTime().After(since) {
			changed = append(changed, name)
		}
	}
	switch len(changed) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("Your %s changed since your last turn; read it again before you answer.\n\n", changed[0])
	}
	return fmt.Sprintf("Your %s changed since your last turn; read them again before you answer.\n\n", strings.Join(changed, " and "))
}

// taskNote shows a bot the open tasks whose line its agent does not hold, by
// the digests in held, with the numbers of the rest; lines are the digests
// of the open tasks' lines the agent holds
// after the note.
func taskNote(store NoteStore, bot string, held map[int64]string) (note string, lines map[int64]string) {
	open, err := store.ListOpenCommitments(bot)
	if err != nil {
		return fmt.Sprintf("Your open tasks could not be read: %v.\n\n", err), held
	}
	lines = make(map[int64]string, len(open))
	var changed strings.Builder
	var unchanged []string
	for _, commitment := range open {
		line, err := taskLine(store, commitment)
		if err != nil {
			return fmt.Sprintf("Your open tasks could not be read: %v.\n\n", err), held
		}
		lines[commitment.ID] = digest([]byte(line))
		if held[commitment.ID] == lines[commitment.ID] {
			unchanged = append(unchanged, fmt.Sprintf("#%d", commitment.ID))
			continue
		}
		changed.WriteString(line + "\n")
	}
	var b strings.Builder
	if changed.Len() > 0 {
		b.WriteString("Your open tasks that are new or changed since you last saw them. Use bot_task_update with status completed and message to deliver results, or cancelled with a reason. Stopped tasks stay dormant until explicitly resumed to pending with a justified message; unrelated messages and stale results never resume them. bot_task_list reads every open task:\n" + changed.String())
		if len(unchanged) > 0 {
			b.WriteString("Unchanged: " + strings.Join(unchanged, ", ") + ".\n")
		}
	}
	if b.Len() == 0 {
		return "", lines
	}
	return b.String() + "\n", lines
}

func taskLine(store PromptStore, commitment storage.Commitment) (string, error) {
	_, name, err := owedTo(store, commitment)
	if err != nil {
		return "", err
	}
	name = cmp.Or(name, "the user")
	var b strings.Builder
	fmt.Fprintf(&b, "#%d [%s] for %s: %s", commitment.ID, commitment.Status, name, commitment.Request)
	for _, thread := range commitment.Threads {
		fmt.Fprintf(&b, " · thread %s %s", thread.Slug, thread.State)
	}
	if commitment.Note != "" {
		label := "next"
		if commitment.Status == storage.CommitmentStopped {
			label = "reason"
		}
		fmt.Fprintf(&b, " · %s: %s", label, commitment.Note)
	}
	b.WriteString(standing(commitment))
	return b.String(), nil
}
