package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionoverview"
	"github.com/wins/jaz/backend/internal/storage"
)

type overviewHandlerStore struct {
	view storage.SessionOverview
}

func (s overviewHandlerStore) LoadSessionOverview(context.Context, string) (storage.SessionOverview, error) {
	return s.view, nil
}

type overviewHandlerLive struct{}

func (overviewHandlerLive) HydrationJobs([]string) map[string]acp.HydrationView { return nil }

func TestOverviewHandlerReturnsFlattenedMetadata(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	store := overviewHandlerStore{view: storage.SessionOverview{
		Threads: []storage.OverviewThread{{
			ID: "child", Slug: "child", ACPAgent: "codex", Status: storage.StatusIdle,
			Archived: true, UpdatedAt: now,
		}},
		SubagentEvents: []sessionevents.Event{{
			Seq: 42, Type: sessionevents.TypeProviderSubagent, At: now,
			ProviderSubagent: &sessionevents.ProviderSubagentEvent{
				Provider: "codex", ID: "/root/newton", ThreadID: "provider-thread", ParentID: "provider-parent",
				Name: "Newton", Status: "completed", StartedAtMs: 1, CompletedAtMs: 2,
			},
		}},
	}}
	handler := NewOverviewHandler(sessionoverview.NewService(store, overviewHandlerLive{}))
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/parent/overview", nil)
	request.SetPathValue("session", "parent")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body overviewResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Threads) != 1 || !body.Threads[0].Archived {
		t.Fatalf("threads = %#v", body.Threads)
	}
	if len(body.Subagents) != 1 || body.Subagents[0].Name != "Newton" || body.Subagents[0].Key == "" || body.Subagents[0].Seq != 42 {
		t.Fatalf("subagents = %#v", body.Subagents)
	}
	var raw struct {
		Subagents []map[string]any `json:"subagents"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"thread_id", "parent_id", "started_at_ms", "completed_at_ms"} {
		if _, ok := raw.Subagents[0][field]; ok {
			t.Fatalf("subagent response leaked %q: %s", field, response.Body.String())
		}
	}
}
