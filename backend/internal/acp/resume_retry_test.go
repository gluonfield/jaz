package acp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestAResumeWhoseAgentDiesOnItsWayUpSucceedsOnTheNextTry(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	jaz.busy(bot.ID)

	killed := filepath.Join(root, "killed")
	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_EXIT_ONCE": killed})
	if _, err := os.Stat(killed); err != nil {
		t.Fatalf("the first try did not start an agent that died: %v", err)
	}
	jaz.eventually("the resumed turn finishing", func() bool {
		session, err := store.LoadSession(bot.ID)
		if err != nil || session.Status == storage.StatusError {
			t.Fatalf("bot = %+v, %v; want its turn resumed on the next try", session, err)
		}
		return session.Status == storage.StatusIdle
	})
	if got := jaz.promptsContaining("Continue from where you left off"); got != 1 {
		t.Fatalf("resumed turns = %d, prompts = %q", got, jaz.prompts())
	}
}
