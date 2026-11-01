package bots

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wins/jaz/backend/internal/storage"
)

const homeInstructionsMigration = "bot_home_instructions_v1"

// MigrateHomes moves legacy bot identities once, before ACP starts workers.
// Later AGENTS.md files are shared workspace instructions, not bot identities.
func MigrateHomes(store interface {
	ListBots() ([]storage.BotRecord, error)
	LoadSession(string) (storage.Session, error)
	LoadSetting(string, string) (storage.Setting, error)
	SaveSetting(string, string, json.RawMessage) (storage.Setting, error)
}) error {
	if _, err := store.LoadSetting("migrations", homeInstructionsMigration); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrSettingNotFound) {
		return err
	}
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
	_, err = store.SaveSetting("migrations", homeInstructionsMigration, json.RawMessage("true"))
	return err
}
