package acp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestBotTaskToolsManageExistingWorkAndDeliverThroughMCP(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	legacy := owe(t, store, bot.ID)
	server := mcp.NewServer(&mcp.Implementation{Name: "jaztools", Version: "1"}, nil)
	bots.NewMCPTools(jaz.bots).AddBotTo(server)
	acp.NewMCPTools(jaz.manager).AddTo(server)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(mcpsession.HeaderName, bot.ID)
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(jaz.ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	call := func(name string, args map[string]any, fails bool) *mcp.CallToolResult {
		t.Helper()
		result, err := client.CallTool(jaz.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		failed := err != nil || result.IsError
		if failed != fails {
			t.Fatalf("%s returned %+v, %v; want failure %v", name, result, err, fails)
		}
		return result
	}
	create := func(request string) int64 {
		t.Helper()
		result := call("bot_task_create", map[string]any{"request": request, "next": "check the result"}, false)
		var task bots.MCPTaskOutput
		decodeMCPTaskResult(t, result, &task)
		return task.ID
	}

	call("bot_task_update", map[string]any{"id": legacy, "next": "deliver the existing result", "waiting": "requester"}, false)
	if task := jaz.commitment(legacy); task.Note != "deliver the existing result" || task.Waiting != storage.WaitingRequester {
		t.Fatalf("existing task was not updated: %+v", task)
	}
	id := create("Compare the two quotes")
	result := call("create_thread", map[string]any{"prompt": "Compare quotes until released", "task": id}, false)
	var worker acp.ThreadSnapshot
	decodeMCPTaskResult(t, result, &worker)
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
	if task := jaz.commitment(id); len(task.Threads) != 1 || task.Threads[0].ID != worker.ThreadID {
		t.Fatalf("worker is not linked to the task: %+v", task)
	}
	call("bot_task_complete", map[string]any{"ids": []int64{id}, "message": "The first quote is cheaper."}, true)
	call("bot_task_cancel", map[string]any{"id": id, "reason": "The user cancelled it"}, true)
	if task := jaz.commitment(id); task.Status != storage.CommitmentOpen {
		t.Fatalf("working task closed: %+v", task)
	}
	jaz.cancel(worker.ThreadID)
	jaz.idle(worker.ThreadID)
	call("bot_task_complete", map[string]any{"ids": []int64{legacy, id}, "message": "The first quote is cheaper."}, false)
	for _, taskID := range []int64{legacy, id} {
		if task := jaz.commitment(taskID); task.Status != storage.CommitmentDone {
			t.Fatalf("delivered task stayed open: %+v", task)
		}
	}
	if !slices.Contains(jaz.notices(bot.ID), "The first quote is cheaper.") {
		t.Fatal("tasks closed without delivering the result")
	}

	cancelled := create("Draft the brochure")
	call("bot_task_cancel", map[string]any{"id": cancelled, "reason": " "}, true)
	call("bot_task_complete", map[string]any{"ids": []int64{}, "message": "Finished"}, true)
	call("bot_task_update", map[string]any{"id": 0, "request": "Unexpected new work"}, true)
	call("bot_task_cancel", map[string]any{"id": cancelled, "reason": "The user cancelled it"}, false)
	if task := jaz.commitment(cancelled); task.Status != storage.CommitmentDropped || task.Note != "The user cancelled it" {
		t.Fatalf("cancelled task = %+v", task)
	}
	if open, err := store.ListOpenCommitments(bot.ID); err != nil || len(open) != 0 {
		t.Fatalf("unexpected open tasks: %+v, %v", open, err)
	}
}

func decodeMCPTaskResult(t *testing.T, result *mcp.CallToolResult, output any) {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatal(err)
	}
}
