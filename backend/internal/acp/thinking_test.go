package acp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
)

// A bot thinks while its agent holds the prompt and has called no tool: not
// once the agent calls one, nor while its turn waits to retry a provider
// failure.
func TestABotThinksOnlyWhileItsAgentReasons(t *testing.T) {
	acp.SetProviderRetryDelays(t, time.Hour)
	store, root := newTestStore(t)
	hold := filepath.Join(root, "thinking")
	env := failingProvider(root)
	env["JAZ_FAKE_ACP_THINK_HOLD"] = hold
	jaz := startJaz(t, testContext(t), store, root, env)
	bot := jaz.newBot("Gimli")
	thinking := func() bool {
		listed, err := jaz.bots.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, other := range listed {
			if other.ID == bot.ID {
				return other.Thinking
			}
		}
		t.Fatal("the bot is not listed")
		return false
	}

	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "plan the trip until released", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the bot thinking", thinking)
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	if thinking() {
		t.Fatal("a bot whose agent called a tool is thinking")
	}
	jaz.release()
	jaz.idle(bot.ID)

	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "book the hotel provider fails 1 overloaded", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.eventually("the turn waiting to retry", func() bool { return strings.Contains(jaz.status(bot.ID).Error, "Retrying in") })
	if thinking() {
		t.Fatal("a bot waiting to retry a provider failure is thinking")
	}
	if _, err := jaz.manager.Cancel(jaz.ctx, bot.ID); err != nil {
		t.Fatal(err)
	}
}
