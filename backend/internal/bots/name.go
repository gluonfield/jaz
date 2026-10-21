package bots

import "github.com/wins/jaz/backend/internal/storage"

// Name reads a bot conversation's current identity; other threads have none.
func Name(store PromptStore, threadID string) (string, error) {
	session, err := store.LoadSession(threadID)
	if err != nil || session.SourceType != storage.SourceBot {
		return "", err
	}
	record, err := store.LoadBot(threadID)
	if err != nil || record.Kind != KindBot {
		return "", err
	}
	return session.Title, nil
}
