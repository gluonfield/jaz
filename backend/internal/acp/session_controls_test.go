package acp

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestAgentConnectionRetainsInitialControlsAndAuth(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "controls", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	state := &connectionState{}
	handler := state.handler(manager, jsonrpc.HandlerFunc(manager.handleJSONRPC))
	notify := func(method, params string) {
		t.Helper()
		if _, err := handler.HandleJSONRPC(context.Background(), jsonrpc.Request{Method: method, Params: json.RawMessage(params)}); err != nil {
			t.Fatal(err)
		}
	}
	notify("_auth/status_update", `{"authStatus":{"kind":"account","label":"ChatGPT Pro","account":{"email":"user@example.test","plan":"pro"}}}`)
	notify("session/update", `{"sessionId":"native","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"compact","description":"Compact context","input":{"hint":"instructions"}}]}}`)
	state.initial = json.RawMessage(`{"configOptions":[{"id":"speed","name":"Speed","type":"select","currentValue":"standard","options":[{"value":"standard","name":"Standard"},{"value":"fast","name":"Fast"}],"_meta":{"jetbrains":{"air":{"version":1,"recommendedValue":"standard"}}}}]}`)
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPSession: "native"}}
	manager.addJob(job, nil)
	state.attach(manager, job, AgentConfig{})
	if hasTranscript, err := store.HasSessionTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("agent metadata materialized a conversation: %t, %v", hasTranscript, err)
	}
	if got := job.agentSession; got.Auth.Account.Plan != "pro" || len(got.Commands) != 1 || got.ConfigOptions[0].RecommendedValue != "standard" {
		t.Fatalf("initial state = %#v", got)
	}
	notify("session/update", `{"sessionId":"native","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"speed","name":"Speed","type":"select","currentValue":"fast","options":[{"group":"speeds","name":"Available speeds","options":[{"value":"fast","name":"Fast"}]}]}]}}`)
	notify("_auth/status_update", `{"authStatus":{"kind":"none","label":"Not logged in"}}`)
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	last.NormalizePayload()
	if last.Type != sessionevents.TypeAgentSession || last.AgentSession.Auth.Kind != "none" || last.AgentSession.Auth.Account != nil {
		t.Fatalf("stored auth state = %#v", last)
	}
	option := last.AgentSession.ConfigOptions[0]
	if option.CurrentValue != "fast" || option.Options[0].Group != "Available speeds" || len(last.AgentSession.Commands) != 1 {
		t.Fatalf("stored controls = %#v", last.AgentSession)
	}
}

func TestBackgroundTaskUpdatesSurviveIdleTurnAndStorage(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "background", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPSession: "native", State: StateIdle}}
	manager.addJob(job, nil)
	for _, raw := range []string{
		`{"sessionUpdate":"async_task_spawned","asyncTaskId":"task-1","name":"Build","taskType":"local_bash","canStop":true,"showInTranscript":true}`,
		`{"sessionUpdate":"async_task_progress","asyncTaskId":"task-1","summary":"Compiling","usage":{"totalTokens":12,"toolUses":2,"durationMs":30}}`,
		`{"sessionUpdate":"async_task_state_update","asyncTaskId":"task-1","state":"completed","summary":"Build succeeded"}`,
	} {
		manager.applyUpdate("native", json.RawMessage(raw))
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("stored task updates = %d, want 3", len(events))
	}
	last := events[len(events)-1]
	last.NormalizePayload()
	task := last.AgentTask
	if task == nil || task.Name != "Build" || task.State != "completed" || task.Summary != "Build succeeded" || task.CanStop || task.Usage.TotalTokens != 12 {
		t.Fatalf("stored task = %#v", task)
	}
}

func TestNativeControlsPersistUserSelectionAndStopTasks(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-controls", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	defer manager.Close()
	left, right := net.Pipe()
	clientConn := stdio.New(left, left)
	remoteConn := stdio.New(right, right)
	defer remoteConn.Close()
	selected := "auto"
	config := func() json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"configOptions": []map[string]any{{
			"id": "mode", "name": "Mode", "type": "select", "category": "mode", "currentValue": selected,
			"options": []map[string]string{{"value": "auto", "name": "Auto"}, {"value": "default", "name": "Manual"}, {"value": "acceptEdits", "name": "Accept edits"}},
		}}})
		return raw
	}
	stopped := ""
	agent := jsonrpc.NewPeer(remoteConn, jsonrpc.HandlerFunc(func(ctx context.Context, req jsonrpc.Request) (json.RawMessage, *jsonrpc.Error) {
		var params struct {
			Value  string `json:"value"`
			ModeID string `json:"modeId"`
			TaskID string `json:"asyncTaskId"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, jsonrpc.InvalidParams(err.Error(), nil)
		}
		switch req.Method {
		case "session/set_config_option":
			selected = params.Value
			if selected == "auto" {
				selected = "acceptEdits"
			}
			return config(), nil
		case "session/set_mode":
			selected = params.ModeID
			return json.RawMessage(`{}`), nil
		case "_session/async_task/stop":
			stopped = params.TaskID
			return json.RawMessage(`{"stopped":true}`), nil
		default:
			return nil, jsonrpc.MethodNotFound(req.Method)
		}
	}))
	peer := jsonrpc.NewPeer(clientConn, jsonrpc.HandlerFunc(manager.handleJSONRPC))
	go agent.Serve(t.Context())
	go peer.Serve(t.Context())
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, ACPAgent: AgentClaude, ACPSession: "native", Modes: ModeState{
		CurrentModeID: "auto", PlanModeID: "plan", AvailableModes: []ModeSnapshot{{ID: "auto"}, {ID: "default"}, {ID: "acceptEdits"}, {ID: "plan"}, {ID: "bypassPermissions"}},
	}}}
	manager.addJob(job, newAgentProcess(&agentConn{conn: clientConn, peer: peer}))
	manager.applySessionControls(job, config())
	if err := manager.SetSessionConfig(t.Context(), session.ID, "mode", "default"); err != nil {
		t.Fatal(err)
	}
	if err := manager.prepareModeForTurn(t.Context(), job, false); err != nil {
		t.Fatal(err)
	}
	if selected != "default" {
		t.Fatalf("turn overwrote user-selected mode with %q", selected)
	}
	if err := manager.prepareModeForTurn(t.Context(), job, true); err != nil {
		t.Fatal(err)
	}
	if selected != "plan" {
		t.Fatalf("planning turn mode = %q", selected)
	}
	manager.applyUpdate("native", config())
	events, err := store.LoadSessionOverviewEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.prepareModeForTurn(t.Context(), job, false); err != nil {
		t.Fatal(err)
	}
	if selected != "default" {
		t.Fatalf("planning turn lost user-selected mode: %q", selected)
	}
	selected = "auto"
	job.agentSession = sessionevents.AgentSession{}
	job.Modes.userModeID = ""
	manager.applySessionControls(job, config())
	manager.restoreSessionControls(t.Context(), job, events)
	if selected != "default" {
		t.Fatalf("reconnect lost user-selected mode: %q", selected)
	}
	selected = "auto"
	manager.applyUpdate("native", config())
	if err := manager.prepareModeForTurn(t.Context(), job, false); err != nil {
		t.Fatal(err)
	}
	if selected != "auto" {
		t.Fatalf("turn overwrote provider-selected mode with %q", selected)
	}
	if err := manager.SetSessionConfig(t.Context(), session.ID, "mode", "auto"); err != nil {
		t.Fatal(err)
	}
	events, err = store.LoadSessionOverviewEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var restoredChoice *string
	for _, event := range events {
		if event.AgentSession != nil {
			restoredChoice = event.AgentSession.ConfigOptions[0].UserValue
		}
	}
	if restoredChoice == nil || *restoredChoice != "acceptEdits" {
		t.Fatalf("provider-adjusted user selection was not saved: %v", restoredChoice)
	}
	manager.applyUpdate("native", json.RawMessage(`{"sessionUpdate":"async_task_spawned","asyncTaskId":"build","name":"Build","canStop":true}`))
	if err := manager.StopBackgroundTask(t.Context(), session.ID, "build"); err != nil {
		t.Fatal(err)
	}
	if stopped != "build" || job.backgroundTasks["build"].State != "running" {
		t.Fatal("stop request did not preserve the native task state until notification")
	}
	manager.disconnectBackgroundTasks(job)
	if task := job.backgroundTasks["build"]; !task.ConnectionLost || !task.CanStop || task.State != "running" {
		t.Fatalf("disconnected task = %#v", task)
	}
	if err := manager.StopBackgroundTask(t.Context(), session.ID, "build"); err == nil {
		t.Fatal("disconnected task accepted a stop request")
	}
	manager.applyUpdate("native", json.RawMessage(`{"sessionUpdate":"async_task_progress","asyncTaskId":"build","summary":"Still compiling"}`))
	if err := manager.StopBackgroundTask(t.Context(), session.ID, "build"); err != nil {
		t.Fatalf("native progress did not restore stop capability: %v", err)
	}
}

func TestModelUpdateInvalidatesPreviousContextLimit(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "model-context", Model: "first", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: session.ID, Slug: session.Slug, Model: "first", ACPSession: "native"}}
	manager.addJob(job, nil)
	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":1000,"size":200000}`))
	manager.applyUpdate("native", json.RawMessage(`{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","type":"select","currentValue":"second","options":[{"value":"second","name":"Second"}]}]}`))
	manager.recordRawUsage(job, json.RawMessage(`{"sessionUpdate":"usage_update","used":1010,"size":0}`))
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.usage.ContextWindowTokens != 0 || loaded.Usage.ContextWindowTokens != 0 || loaded.Usage.ContextTokens != 1010 {
		t.Fatalf("model change retained prior capacity: runtime=%#v stored=%#v", job.usage, loaded.Usage)
	}
}

func TestProviderChangeSupersedesSavedChoice(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "provider-choice", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	explicit := "high"
	job := &jobState{Job: Job{ID: session.ID}, agentSession: sessionevents.AgentSession{ConfigOptions: []sessionevents.AgentConfigOption{{ID: "effort", Category: "thought_level", CurrentValue: "high", UserValue: &explicit}}}}
	for _, current := range []string{"high", "medium"} {
		raw, err := json.Marshal(map[string]any{"configOptions": []map[string]any{{"id": "effort", "category": "thought_level", "type": "select", "currentValue": current, "options": []map[string]string{{"value": "high"}, {"value": "medium"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		manager.applySessionControls(job, raw)
		choice := job.agentSession.ConfigOptions[0].UserValue
		if (choice != nil) != (current == "high") {
			t.Fatalf("reported effort %q retained choice %v", current, choice)
		}
	}
}

func TestWithdrawnControlsClearStoredModelMetadata(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "withdrawn-controls", Runtime: storage.RuntimeACP, Model: "legacy-model", ReasoningEffort: "legacy-effort"})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: jobFromSession(session, AgentClaude, "native", "", StateIdle)}
	manager.addJob(job, nil)
	manager.applyUpdate("native", json.RawMessage(`{"configOptions":[]}`))
	if job.Model != "legacy-model" || job.ReasoningEffort != "legacy-effort" {
		t.Fatal("unrelated config options replaced legacy model metadata")
	}
	manager.applyUpdate("native", json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"native-model"},{"id":"effort","category":"thought_level","type":"select","currentValue":"high"}]}`))
	manager.applyUpdate("native", json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"native-model"}]}`))
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.ReasoningEffort != "" || loaded.ReasoningEffort != "" || loaded.Model != "native-model" {
		t.Fatalf("withdrawn effort retained stale metadata: runtime=%q stored=%#v", job.ReasoningEffort, loaded)
	}
	manager.applyUpdate("native", json.RawMessage(`{"configOptions":[]}`))
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Model != "" || loaded.Model != "" {
		t.Fatalf("withdrawn model retained stale metadata: runtime=%q stored=%q", job.Model, loaded.Model)
	}
}

func TestProviderModeChangeUpdatesPinnedBaseline(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-mode", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	job := &jobState{Job: Job{ID: session.ID, ACPAgent: AgentClaude, Modes: ModeState{userModeID: "auto", PlanModeID: "plan"}}}
	for _, mode := range []string{"plan", "default"} {
		raw, err := json.Marshal(map[string]any{"configOptions": []map[string]any{{"id": "mode", "category": "mode", "type": "select", "currentValue": mode}}})
		if err != nil {
			t.Fatal(err)
		}
		manager.applySessionControls(job, raw)
		want := "auto"
		if mode == "default" {
			want = mode
		}
		if got := baselineModeID(AgentClaude, job.Modes); got != want {
			t.Fatalf("native mode %s: next-turn baseline = %s, want %s", mode, got, want)
		}
	}
}
