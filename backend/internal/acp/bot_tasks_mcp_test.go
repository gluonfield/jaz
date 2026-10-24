package acp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

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
	jaz.write(bot.ID, "Compare the supplier quotes.")
	jaz.settles(bot.ID, 0, 0)
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

	call("bot_task_update", map[string]any{"id": legacy, "next": "deliver the existing result", "status": "waiting_for_user", "message": "Which supplier should I use?"}, false)
	if task := jaz.commitment(legacy); task.Note != "deliver the existing result" || task.Status != storage.CommitmentWaitingForUser || task.QuestionSeq == 0 {
		t.Fatalf("existing task was not updated: %+v", task)
	}
	listed := call("bot_task_list", map[string]any{}, false)
	var tasks bots.MCPTasksOutput
	decodeMCPTaskResult(t, listed, &tasks)
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].ID != legacy || tasks.Tasks[0].Status != storage.CommitmentWaitingForUser {
		t.Fatalf("unfinished tasks = %+v", tasks.Tasks)
	}
	question := jaz.commitment(legacy).QuestionSeq
	call("bot_task_update", map[string]any{"id": legacy, "status": "pending", "answered_question": question}, true)
	otherWait := create("Book the courier")
	call("bot_task_update", map[string]any{"id": otherWait, "status": "waiting_for_user", "message": "Which delivery address?"}, false)
	jaz.write(bot.ID, "Supplier B")
	jaz.settles(bot.ID, 0, 0)
	call("bot_task_update", map[string]any{"id": legacy, "status": "pending"}, true)
	call("bot_task_update", map[string]any{"id": legacy, "status": "pending", "answered_question": question + 1}, true)
	call("bot_task_update", map[string]any{"id": legacy, "status": "pending", "answered_question": question}, false)
	if task := jaz.commitment(otherWait); task.Status != storage.CommitmentWaitingForUser {
		t.Fatalf("an unrelated question was released: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": otherWait, "status": "waiting_for_user", "message": "Please confirm the address."}, false)
	if task := jaz.commitment(otherWait); task.ReplySeq != 0 {
		t.Fatalf("a replacement question reused an old answer: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": otherWait, "status": "pending", "answered_question": jaz.commitment(otherWait).QuestionSeq}, true)
	call("bot_task_update", map[string]any{"id": otherWait, "status": "cancelled", "message": "The courier is no longer needed."}, false)
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	call("bot_task_update", map[string]any{"id": legacy, "check_at": "tomorrow"}, true)
	call("bot_task_update", map[string]any{"id": legacy, "check_at": future.Format(time.RFC3339)}, false)
	call("bot_task_update", map[string]any{"id": legacy, "next": "Check the supplier response"}, false)
	if task := jaz.commitment(legacy); task.CheckAt == nil || !task.CheckAt.Equal(future) {
		t.Fatalf("progress lost the scheduled check: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": legacy, "check_at": ""}, false)
	if task := jaz.commitment(legacy); task.CheckAt != nil {
		t.Fatalf("explicit clear kept a future check: %+v", task)
	}
	id := create("Compare the two quotes")
	dependent := create("Prepare the purchase summary")
	call("bot_task_update", map[string]any{"id": dependent, "depends_on": []int64{id}}, false)
	call("bot_task_update", map[string]any{"id": dependent, "next": "Wait for the comparison"}, false)
	if task := jaz.commitment(dependent); task.Status != storage.CommitmentPending || !slices.Equal(task.DependsOn, []int64{id}) || task.Request != "Prepare the purchase summary" {
		t.Fatalf("omitted task fields changed: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": dependent, "depends_on": []int64{}}, false)
	if task := jaz.commitment(dependent); task.Blocked || len(task.DependsOn) != 0 {
		t.Fatalf("explicit empty dependencies did not clear them: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": dependent, "depends_on": []int64{id}}, false)
	call("create_thread", map[string]any{"prompt": "Start the dependent work", "task": dependent}, true)
	call("bot_task_update", map[string]any{"id": dependent, "status": "waiting_for_user", "message": "This question must not be delivered.", "depends_on": []int64{dependent}}, true)
	if task := jaz.commitment(dependent); task.Status != storage.CommitmentPending || !task.Blocked || task.QuestionSeq != 0 {
		t.Fatalf("failed question update changed the task: %+v", task)
	}
	if slices.Contains(jaz.notices(bot.ID), "This question must not be delivered.") {
		t.Fatal("a failed task update leaked its question")
	}
	call("bot_task_update", map[string]any{"id": dependent, "status": "cancelled", "message": "The summary is no longer required."}, false)
	result := call("create_thread", map[string]any{"prompt": "Compare quotes until released", "task": id}, false)
	var worker acp.ThreadSnapshot
	decodeMCPTaskResult(t, result, &worker)
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
	if task := jaz.commitment(id); len(task.Threads) != 1 || task.Threads[0].ID != worker.ThreadID {
		t.Fatalf("worker is not linked to the task: %+v", task)
	}
	call("bot_task_update", map[string]any{"id": id, "status": "completed", "message": "The first quote is cheaper."}, true)
	call("bot_task_update", map[string]any{"id": id, "status": "cancelled", "message": "The user cancelled it"}, true)
	if task := jaz.commitment(id); !task.IsOpen() {
		t.Fatalf("working task closed: %+v", task)
	}
	if slices.Contains(jaz.notices(bot.ID), "The first quote is cheaper.") {
		t.Fatal("a refused completion delivered its result")
	}
	jaz.cancel(worker.ThreadID)
	jaz.idle(worker.ThreadID)
	call("bot_task_update", map[string]any{"id": legacy, "status": "completed", "message": "The supplier comparison is ready."}, false)
	call("bot_task_update", map[string]any{"id": id, "status": "completed", "message": "The first quote is cheaper."}, false)
	for _, taskID := range []int64{legacy, id} {
		if task := jaz.commitment(taskID); task.Status != storage.CommitmentDone {
			t.Fatalf("delivered task stayed open: %+v", task)
		}
	}
	if !slices.Contains(jaz.notices(bot.ID), "The first quote is cheaper.") {
		t.Fatal("tasks closed without delivering the result")
	}

	cancelled := create("Draft the brochure")
	call("bot_task_update", map[string]any{"id": cancelled, "status": "cancelled", "message": " "}, true)
	call("bot_task_update", map[string]any{"id": cancelled, "status": "waiting_for_user"}, true)
	call("bot_task_update", map[string]any{"id": 0, "request": "Unexpected new work"}, true)
	call("bot_task_update", map[string]any{"id": cancelled, "status": "cancelled", "message": "The user cancelled it"}, false)
	if task := jaz.commitment(cancelled); task.Status != storage.CommitmentDropped || task.Note != "The user cancelled it" {
		t.Fatalf("cancelled task = %+v", task)
	}
	if open, err := store.ListOpenCommitments(bot.ID); err != nil || len(open) != 0 {
		t.Fatalf("unexpected open tasks: %+v, %v", open, err)
	}
	decodeMCPTaskResult(t, call("bot_task_list", map[string]any{}, false), &tasks)
	if len(tasks.Tasks) != 0 {
		t.Fatalf("closed tasks leaked into working context: %+v", tasks.Tasks)
	}
	view, err := jaz.bots.Activity(bot.ID)
	if err != nil || len(view.Items) != 0 || len(view.Owed) != 0 {
		t.Fatalf("closed tasks leaked into Activity: %+v, %v", view, err)
	}
	for range 101 {
		if _, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Unfinished work"}); err != nil {
			t.Fatal(err)
		}
	}
	other := jaz.newBot("Other")
	if _, err := store.OpenCommitment(storage.Commitment{BotID: other.ID, Request: "Another bot's work"}); err != nil {
		t.Fatal(err)
	}
	decodeMCPTaskResult(t, call("bot_task_list", map[string]any{}, false), &tasks)
	if len(tasks.Tasks) != 101 {
		t.Fatalf("task list was truncated or crossed bot ownership: %d tasks", len(tasks.Tasks))
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
