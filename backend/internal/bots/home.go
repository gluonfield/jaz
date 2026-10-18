package bots

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// A bot's identity lives in two files in its home: AGENTS.md, who it is and
// how it works, which agents load themselves when a session starts, and
// MEMORY.md, what it knows, which Jaz adds to its prompt.
const (
	agentsFile = "AGENTS.md"
	memoryFile = "MEMORY.md"
)

var homeFiles = []string{agentsFile, memoryFile}

// fileLimit bounds how much of an identity file a prompt carries, in
// characters.
const fileLimit = 8000

// homeModule is one of the identity files in bot's home as a prompt module.
func homeModule(bot storage.Session, name string) string {
	if bot.RuntimeRef == nil || bot.RuntimeRef.Cwd == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(bot.RuntimeRef.Cwd, name))
	text := strings.TrimSpace(string(data))
	if err != nil || text == "" {
		return ""
	}
	if runes := []rune(text); len(runes) > fileLimit {
		text = string(runes[:fileLimit]) + fmt.Sprintf("\n\n[%s continues past %d characters and is cut here. Read the rest from the file and shorten it.]", name, fileLimit)
	}
	return fmt.Sprintf("## Your %s\n\n%s", name, text)
}

// ErrFileChanged refuses a save made over a version of the file that is no
// longer current.
var ErrFileChanged = errors.New("the file changed since it was read")

// File is one of a bot's identity files. Version is a digest of the content
// read, so a save over it can tell whether the file changed in between; a
// file not yet written has none.
type File struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

func (s *Service) Files(botID string) ([]File, error) {
	home, err := s.home(botID)
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(homeFiles))
	for _, name := range homeFiles {
		file, err := readFile(home, name)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// SaveFile writes the user's edit to one of a bot's identity files, unless
// the file has moved on from version, as when the bot wrote to it meanwhile.
// The edit replaces the file whole, so an agent reading it never sees half of
// it; a write by the bot between the check and the replace is still lost,
// since the bot writes with its own tools.
func (s *Service) SaveFile(botID, name, content, version string) (File, error) {
	if !slices.Contains(homeFiles, name) {
		return File{}, fmt.Errorf("unknown file %q", name)
	}
	home, err := s.home(botID)
	if err != nil {
		return File{}, err
	}
	current, err := readFile(home, name)
	if err != nil {
		return File{}, err
	}
	if current.Version != version {
		return current, ErrFileChanged
	}
	if err := replaceFile(filepath.Join(home, name), content); err != nil {
		return File{}, err
	}
	saved, err := readFile(home, name)
	if err != nil {
		return File{}, err
	}
	s.record(botID, sessionevents.BotActivityEvent{Kind: "file_edited", Label: name}, saved.UpdatedAt)
	return saved, nil
}

// NoteHomeChanges records in a bot's thread the changes to its identity files
// not recorded yet, as a turn in one of the bot's threads ends. Who made them
// is unknown: the user's own edits are recorded as they are saved.
func (s *Service) NoteHomeChanges(threadID string) {
	bot := s.BotOf(threadID)
	if bot == "" {
		return
	}
	home, err := s.home(bot)
	if err != nil {
		return
	}
	since := time.Now().Add(-activityWindow)
	events, err := s.store.LoadBotActivity(bot, since)
	if err != nil {
		s.log.Warn("load bot activity failed", "bot", bot, "error", err)
		return
	}
	recorded := map[string]time.Time{}
	for _, event := range events {
		if activity := event.BotActivity; activity != nil && (activity.Kind == "file_changed" || activity.Kind == "file_edited") {
			recorded[activity.Label] = event.At
		}
	}
	for _, name := range homeFiles {
		info, err := os.Stat(filepath.Join(home, name))
		if err != nil {
			continue
		}
		// Events keep milliseconds, so a change is compared at that precision;
		// one older than Activity looks back is not worth recording.
		changed := info.ModTime().Truncate(time.Millisecond)
		if changed.After(recorded[name]) && changed.After(since) {
			s.record(bot, sessionevents.BotActivityEvent{Kind: "file_changed", Label: name}, info.ModTime())
		}
	}
}

func (s *Service) home(botID string) (string, error) {
	record, session, err := s.load(botID)
	if err != nil {
		return "", err
	}
	if record.Kind != KindBot || session.RuntimeRef == nil || session.RuntimeRef.Cwd == "" {
		return "", fmt.Errorf("%s has no home", botID)
	}
	return session.RuntimeRef.Cwd, nil
}

func readFile(home, name string) (File, error) {
	path := filepath.Join(home, name)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Name: name}, nil
	}
	if err != nil {
		return File{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)
	return File{Name: name, Content: string(data), Version: hex.EncodeToString(sum[:8]), UpdatedAt: info.ModTime().UTC()}, nil
}

// replaceFile writes content beside path and renames it over path.
func replaceFile(path, content string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// TurnNote opens a turn in a bot's own thread with its open commitments,
// and there or in one of its group threads with a note to reread the identity
// files that changed since its last successful turn started. Agents load them
// as a session starts, not every turn, and Codex keeps the prompt its thread
// started with.
func TurnNote(store NoteStore) func(storage.Session) string {
	return func(thread storage.Session) string {
		note := changedFilesNote(thread)
		if thread.SourceType == storage.SourceBot {
			note += commitmentNote(store, thread.ID)
		}
		return note
	}
}

func changedFilesNote(thread storage.Session) string {
	if thread.SourceType != storage.SourceBot && thread.SourceType != storage.SourceBotMember || thread.RuntimeRef == nil || thread.RuntimeRef.Cwd == "" {
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
