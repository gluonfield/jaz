package acp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	botapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestStoppedTaskSurvivesFollowupsUserMessagesQueuesAndJazRestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	bot := jaz.newBot("Research")
	jaz.write(bot.ID, "Ready for a route review")
	jaz.settles(bot.ID, 0, 0)
	tasks := bots.NewMCPTools(jaz.bots)
	_, created, err := tasks.CreateTask(jaz.ctx, callAs(bot.ID), bots.MCPTaskCreateInput{Request: "Find a usable route"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.ID
	_, worker, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "route", Prompt: "Check the route until released", Commitment: id})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
	reason := "Checked the documented routes; neither supports this input. No useful retry condition is known."
	update := func(status, message string, wantError bool) {
		t.Helper()
		_, _, err := tasks.UpdateTask(jaz.ctx, callAs(bot.ID), bots.MCPTaskUpdateInput{ID: id, Status: status, Message: message})
		if (err != nil) != wantError {
			t.Fatalf("task %s = %v, want failure %v", status, err, wantError)
		}
	}
	update(storage.CommitmentStopped, reason, true)
	jaz.cancel(worker.ThreadID)
	jaz.idle(worker.ThreadID)
	update(storage.CommitmentStopped, reason, false)
	jaz.idle(bot.ID)
	wakesBeforeStop := jaz.promptsContaining(owedTurn)
	if _, err := jaz.manager.Input(jaz.ctx, acp.SteerRequest{Session: worker.ThreadID, Message: "Old result says continue"}); err == nil {
		t.Fatal("direct followup started dormant work")
	}
	if _, _, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(worker.ThreadID), acp.MCPCreateInput{Prompt: "Start an unlinked child"}); err == nil {
		t.Fatal("dormant owner created a new child")
	}
	if err := store.QueueTurn(storage.TurnDelivery{SessionID: worker.ThreadID, Message: storage.QueuedMessage{Text: "Old task continuation", From: bot.ID}}); err != nil {
		t.Fatal(err)
	}
	jaz.server.HandleACPTurnFinished(jaz.ctx, acp.Job{ID: worker.ThreadID, State: acp.StateCancelled, StopReason: acp.StopReasonServerShutdown})
	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_UNIQUE_SESSION": "1"})
	tasks = bots.NewMCPTools(jaz.bots)
	if session := jaz.session(worker.ThreadID); session.Status != storage.StatusInterrupted || len(session.QueuedMessages) != 1 {
		t.Fatalf("restart changed the dormant queue: %+v", session)
	}
	if jaz.promptsContaining("Old task continuation") != 0 {
		t.Fatal("restart replayed dormant queued work")
	}
	jaz.write(bot.ID, "How is the weather?")
	jaz.settles(bot.ID, wakesBeforeStop, 0)
	if task := jaz.commitment(id); task.Status != storage.CommitmentStopped || task.Note != reason {
		t.Fatalf("unrelated user message resumed task: %+v", task)
	}
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/bots/{bot}/activity", botapi.NewHandler(jaz.bots, jaz.events).Activity)
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/bots/"+bot.ID+"/activity", nil))
	var activity bots.ActivityView
	decodeErr := json.NewDecoder(response.Body).Decode(&activity)
	if response.Code != http.StatusOK || decodeErr != nil || len(activity.Owed) != 1 || activity.Owed[0].Status != storage.CommitmentStopped || activity.Owed[0].Next != reason {
		t.Fatalf("stopped task projection = %d %s", response.Code, response.Body.String())
	}
	for _, status := range []string{storage.CommitmentInProgress, storage.CommitmentDone, storage.CommitmentWaitingForUser} {
		update(status, "A stale worker says to continue", true)
	}
	update(storage.CommitmentPending, "", true)
	jaz.cancel(worker.ThreadID)
	update(storage.CommitmentPending, "The user supplied a supported input. Review it with the revised instructions.", false)
	if _, err := jaz.manager.Send(jaz.ctx, acp.SendRequest{Session: worker.ThreadID, Message: "Fresh reviewed instructions", Completion: acp.CompletionAsync}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(worker.ThreadID)
	if jaz.promptsContaining("Old task continuation") != 0 || jaz.promptsContaining("Fresh reviewed instructions") != 1 {
		t.Fatal("resume replayed old instructions or missed the new instructions")
	}
	update(storage.CommitmentStopped, reason, false)
	update(storage.CommitmentDropped, "The user has abandoned the request.", false)
	if task := jaz.commitment(id); task.Status != storage.CommitmentDropped {
		t.Fatalf("explicit cancellation failed: %+v", task)
	}
}
