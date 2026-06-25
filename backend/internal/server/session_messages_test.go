package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionMessagesErrorSessionOverridesRunningACPSnapshot(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-restarted",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveACPState(session.ID, storage.ACPState{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateRunning,
	}); err != nil {
		t.Fatal(err)
	}
	session, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	session.Status = storage.StatusError
	session.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:         session.ID,
		Slug:       session.Slug,
		ACPAgent:   "codex",
		ACPSession: "acp-session",
		State:      acp.StateRunning,
	}}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		Session  storage.Session `json:"session"`
		ACPState string          `json:"acp_state"`
		ACPError string          `json:"acp_error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Session.Status != storage.StatusError || got.ACPState != acp.StateFailed {
		t.Fatalf("status = %q, acp_state = %q", got.Session.Status, got.ACPState)
	}
	if got.ACPError != session.Error {
		t.Fatalf("acp_error = %q, want %q", got.ACPError, session.Error)
	}
}

func TestSessionMessagesIncludesActiveOperation(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-compacting",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{job: acp.Job{
		ID:              session.ID,
		Slug:            session.Slug,
		ACPAgent:        "codex",
		ACPSession:      "acp-session",
		State:           acp.StateRunning,
		ActiveOperation: acp.ActiveOperationCompact,
	}}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ActiveOperation string `json:"acp_active_operation"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveOperation != acp.ActiveOperationCompact {
		t.Fatalf("active operation = %q, want compact", got.ActiveOperation)
	}
}

func TestSessionMessagesErrorChildPreservesParentVisibility(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{
		Slug:    "parent",
		Runtime: storage.RuntimeACP,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{
		Slug:     "child",
		Title:    "Child task",
		ParentID: parent.ID,
		Runtime:  storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveACPState(child.ID, storage.ACPState{
		ID:            child.ID,
		Slug:          child.Slug,
		Title:         child.Title,
		ParentID:      parent.ID,
		ACPAgent:      "codex",
		ACPSession:    "acp-session",
		State:         acp.StateRunning,
		ParentVisible: true,
		Assistant:     "private child output",
		Thought:       "private child thought",
		Plan:          []sessionevents.ACPPlanEntry{{Content: "Inspect current page", Status: "completed"}},
		ToolCalls:     []sessionevents.ACPToolCall{{ID: "tool-1", Title: "read file"}},
		Permissions: []sessionevents.ACPPermission{{
			ID:     "perm-1",
			Status: "pending",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	child, err = store.LoadSession(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	child.Status = storage.StatusError
	child.Error = "Server restarted while this thread was still running."
	if err := store.SaveSession(child); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+parent.ID+"/messages", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: &fakeACPManager{
		job: acp.Job{State: acp.StateNotRunning},
		jobs: []acp.Job{{
			ID:            child.ID,
			Slug:          child.Slug,
			Title:         child.Title,
			ParentID:      parent.ID,
			ACPAgent:      "codex",
			ACPSession:    "acp-session",
			State:         acp.StateRunning,
			ParentVisible: true,
		}},
	}}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var got struct {
		ACPChildren []storage.ACPState `json:"acp_children"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ACPChildren) != 1 {
		t.Fatalf("children = %#v", got.ACPChildren)
	}
	state := got.ACPChildren[0]
	if state.ID != child.ID || state.State != acp.StateFailed || state.Error != child.Error {
		t.Fatalf("child state = %#v", state)
	}
	if !state.ParentVisible {
		t.Fatalf("parent_visible = false, want true")
	}
	if state.Assistant != "" || state.Thought != "" || len(state.Plan) != 0 || len(state.ToolCalls) != 0 || len(state.Permissions) != 0 {
		t.Fatalf("child transcript leaked into parent response: %#v", state)
	}
}
