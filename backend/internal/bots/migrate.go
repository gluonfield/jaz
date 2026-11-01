package bots

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/storage"
)

// MigrateHomes moves bot identities out of the project instructions native
// agents automatically inherit. It runs before the ACP manager can start workers.
func MigrateHomes(store interface {
	ListBots() ([]storage.BotRecord, error)
	LoadSession(string) (storage.Session, error)
}) error {
	records, err := store.ListBots()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Kind != KindBot {
			continue
		}
		bot, err := store.LoadSession(record.ThreadID)
		if err != nil {
			return err
		}
		if bot.RuntimeRef == nil || bot.RuntimeRef.Cwd == "" {
			continue
		}
		destination := filepath.Join(bot.RuntimeRef.Cwd, instructionsFile)
		if _, err := os.Lstat(destination); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		old := filepath.Join(bot.RuntimeRef.Cwd, "AGENTS.md")
		if _, err := os.Lstat(old); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Rename(old, destination); err != nil {
			return fmt.Errorf("migrate bot %s instructions: %w", bot.ID, err)
		}
	}
	return nil
}
