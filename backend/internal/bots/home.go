package bots

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	instructionsFile = "BOTS.md"
	memoryFile       = "MEMORY.md"
)

var homeFiles = []string{instructionsFile, memoryFile}

// ErrFileChanged refuses a save made over a version of the file that is no
// longer current.
var ErrFileChanged = instructions.ErrConflict

// File is one of a bot's identity files. Version is a digest of the content
// read, so a save over it can tell whether the file changed in between; a
// file not yet written has none.
type File struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// HomeResolver returns a bot-home lookup backed by store for instructions.LocalFSStore.
func HomeResolver(store PromptStore) func(string) (string, error) {
	return func(botID string) (string, error) {
		record, err := store.LoadBot(botID)
		if err != nil {
			return "", err
		}
		session, err := store.LoadSession(botID)
		if err != nil {
			return "", err
		}
		if session.Archived {
			return "", storage.ErrBotNotFound
		}
		if record.Kind != KindBot || session.RuntimeRef == nil || session.RuntimeRef.Cwd == "" {
			return "", fmt.Errorf("%s has no home", botID)
		}
		return session.RuntimeRef.Cwd, nil
	}
}

func (s *Service) Files(botID string) ([]File, error) {
	files := make([]File, 0, len(homeFiles))
	for _, name := range homeFiles {
		kind, err := instructionKindForFile(name)
		if err != nil {
			return nil, err
		}
		doc, err := s.instructions.Get(context.Background(), instructions.Key{
			Scope:   instructions.ScopeBot,
			ScopeID: botID,
			Kind:    kind,
		})
		if err != nil {
			return nil, err
		}
		files = append(files, fileFromDocument(name, doc))
	}
	return files, nil
}

// SaveFile writes the user's edit to one of a bot's identity files, unless
// the file has moved on from version, as when the bot wrote to it meanwhile.
func (s *Service) SaveFile(botID, name, content, version string) (File, error) {
	kind, err := instructionKindForFile(name)
	if err != nil {
		return File{}, err
	}
	doc, err := s.instructions.Save(context.Background(), instructions.Key{
		Scope:   instructions.ScopeBot,
		ScopeID: botID,
		Kind:    kind,
	}, content, version)
	if errors.Is(err, instructions.ErrConflict) {
		return fileFromDocument(name, doc), ErrFileChanged
	}
	if err != nil {
		return File{}, err
	}
	saved := fileFromDocument(name, doc)
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
	return HomeResolver(s.store)(botID)
}

func instructionKindForFile(name string) (instructions.Kind, error) {
	switch name {
	case instructionsFile:
		return instructions.KindBots, nil
	case memoryFile:
		return instructions.KindMemory, nil
	default:
		return "", fmt.Errorf("unknown file %q", name)
	}
}

func fileFromDocument(name string, doc instructions.Document) File {
	return File{
		Name:      name,
		Content:   doc.Content,
		Version:   doc.Version,
		UpdatedAt: doc.UpdatedAt,
	}
}
