package app

import (
	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/memorysearch"
	"github.com/wins/jaz/backend/internal/memoryservice"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func ConfigureMemorySearchRunner(memory *memoryservice.Service, store *sqlitestore.Store, manager *acp.Manager, logger *log.Logger) {
	if memory == nil || store == nil || manager == nil {
		return
	}
	memory.SetAgenticSearcher(memorysearch.New(store, manager))
}
