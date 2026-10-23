package acp_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const finalReply = "The last commit added due dates."

func TestABotShowsTheUserTheFinalReplyOfTheirTurnThatShowedThemNothing(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")

	// The line before a tool call, the note before the answer and a notice
	// the provider sends as the turn ends are not the reply.
	jaz.write(bot.ID, "what changed? answer in the final reply, provider notice Usage is high.")
	if notices := jaz.notices(bot.ID); !slices.Equal(notices, []string{finalReply}) {
		t.Fatalf("notices = %q, want the turn's final reply", notices)
	}
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	message := slices.IndexFunc(events, func(event sessionevents.Event) bool { return event.RoomMessage != nil })
	ended := len(events) - 1
	for ended >= 0 && (events[ended].Type != "acp" || events[ended].ACP.ID != bot.ID || events[ended].ACP.State != acp.StateIdle) {
		ended--
	}
	if message < 0 || ended < message {
		t.Fatalf("message at %d, turn end at %d; want the reply shown before the turn reads as ended", message, ended)
	}

	// A turn whose last words came before a tool call has no final reply.
	jaz.write(bot.ID, "plan the week")
	jaz.write(bot.ID, "answer in the final reply, commentary only")
	// A turn that sent the user a message shows only that.
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "answer in the final reply until released", Completion: acp.CompletionAsync, FromUser: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	if err := jaz.bots.Say(bot.ID, "Sent the summary."); err != nil {
		t.Fatal(err)
	}
	jaz.release()
	jaz.idle(bot.ID)
	// Turns the user did not start keep their final reply private: another
	// bot's answer coming back, and an agent's message.
	answer := storage.NewInternalQueuedMessage("[reply from Legolas] answer in the final reply")
	answer.Output = &storage.TurnOutput{Hops: 1}
	if err := jaz.server.QueueInternalTurn(jaz.ctx, storage.TurnDelivery{SessionID: bot.ID, Message: answer}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(bot.ID)
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "answer in the final reply", Completion: acp.CompletionAsync, ParentVisible: true}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(bot.ID)
	if notices := jaz.notices(bot.ID); jaz.promptsContaining("answer in the final reply") != 5 || !slices.Equal(notices, []string{finalReply, "Sent the summary."}) {
		t.Fatalf("notices = %q, want nothing from turns without a final reply, a turn that sent one or turns the user did not start", notices)
	}
	jaz.write(bot.ID, "answer in the final reply, unclassified reply")
	if notices := jaz.notices(bot.ID); !slices.Equal(notices, []string{finalReply, "Sent the summary.", finalReply}) {
		t.Fatalf("notices = %q, want the final reply from an agent without phase metadata", notices)
	}
}

func TestABotsFinalReplyReachesTheUserWhenTheirTurnResumesAfterARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "what changed? until released", Completion: acp.CompletionAsync, FromUser: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)

	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_FINAL_REPLY": "1"})
	jaz.idle(bot.ID)
	if notices := jaz.notices(bot.ID); jaz.promptsContaining("Continue from where you left off") != 1 || !slices.Equal(notices, []string{finalReply}) {
		t.Fatalf("notices = %q, prompts = %q; want the resumed turn's final reply", notices, jaz.prompts())
	}
}

func TestABotsFallbackRecordsDeliveryWithoutClosingCommitmentsAndRespectsPresentedApps(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare the quotes", Waiting: storage.WaitingRequester})
	if err != nil {
		t.Fatal(err)
	}
	shown := make(chan bool, 2)
	jaz.manager.TurnEnding = func(job acp.Job) {
		jaz.bots.TurnEnding(job)
		session, err := store.LoadSession(job.ID)
		shown <- err == nil && session.Turn != nil && session.Turn.Output != nil && session.Turn.Output.Shown
	}
	jaz.write(bot.ID, "answer in the final reply")
	if !<-shown || jaz.commitment(commitment.ID).Status != storage.CommitmentOpen {
		t.Fatal("fallback must mark the turn shown and leave the commitment open")
	}
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: bot.ID, Message: "answer in the final reply until released", Completion: acp.CompletionAsync, FromUser: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, bot.ID)
	if err := store.AppendSessionEvents(bot.ID, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeMCPApp, MCPApp: &sessionevents.MCPAppEvent{Presented: true}}); err != nil {
		t.Fatal(err)
	}
	jaz.release()
	jaz.idle(bot.ID)
	if !<-shown || !slices.Equal(jaz.notices(bot.ID), []string{finalReply}) {
		t.Fatal("a presented app must suppress the fallback")
	}
}

func TestABotHearsItsThreadsResultWithoutTheNotesItsProviderMarksInterim(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	jaz.busy(bot.ID)
	if _, _, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "changes", Prompt: "what changed? answer in the final reply", Commitment: owe(t, store, bot.ID)}); err != nil {
		t.Fatal(err)
	}
	jaz.release()
	jaz.eventually("the thread's result reaching the bot", func() bool {
		return jaz.promptWith("ACP session changes") != ""
	})
	if result := jaz.promptWith("ACP session changes"); !strings.Contains(result, finalReply) || strings.Contains(result, "Let me check the log.") || strings.Contains(result, "Found it.") {
		t.Fatalf("the bot was handed %q, want the thread's result without its interim notes", result)
	}
}
