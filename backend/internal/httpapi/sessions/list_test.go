package sessions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestListSearchAndPagination(t *testing.T) {
	for _, adapter := range []string{"sqlite", "json"} {
		t.Run(adapter, func(t *testing.T) {
			var store storage.SessionStore
			if adapter == "sqlite" {
				db, err := sqlitestore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				store = db
			} else {
				db, err := jsonstore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				store = db
			}
			at := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
			for i, title := range []string{"Recent", "Recent child", "Needle 100%", "Older child", "Active", "Loop"} {
				session := storage.Session{
					ID: fmt.Sprint(i), Slug: fmt.Sprintf("thread-%d", i), Title: title,
					Runtime: storage.RuntimeACP, Status: storage.StatusIdle,
					Archived: i != 4, CreatedAt: at, UpdatedAt: at, LastAttentionAt: at.Add(time.Duration(i) * time.Microsecond),
				}
				if i == 1 || i == 3 {
					session.ParentID = "0"
				}
				if i == 2 || i == 3 {
					session.LastAttentionAt = session.LastAttentionAt.Add(-time.Hour)
				}
				if i == 5 {
					session.SourceType = storage.SourceLoopRun
				}
				if err := store.SaveSession(session); err != nil {
					t.Fatal(err)
				}
			}
			handler := NewListHandler(store)
			fetch := func(query string, want ...string) listResponse {
				t.Helper()
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/sessions?"+query, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d: %s", response.Code, response.Body.String())
				}
				var body listResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				var ids []string
				for _, session := range body.Sessions {
					ids = append(ids, session.ID)
				}
				if !reflect.DeepEqual(ids, want) {
					t.Fatalf("query %q returned %v, want %v", query, ids, want)
				}
				return body
			}
			const base = "archived=true&include_children=true"
			first := fetch(base+"&limit=1", "0")
			if first.NextCursor == "" {
				t.Fatal("missing next cursor")
			}
			second := fetch(base+"&limit=1&cursor="+url.QueryEscape(first.NextCursor), "1")
			third := fetch(base+"&limit=1&cursor="+url.QueryEscape(second.NextCursor), "2")
			last := fetch(base+"&limit=1&cursor="+url.QueryEscape(third.NextCursor), "3")
			if last.NextCursor != "" {
				t.Fatalf("last page cursor = %q", last.NextCursor)
			}
			fetch(base+"&limit=2&q="+url.QueryEscape("  NEEDLE  "), "2")
			fetch(base+"&limit=2&q=%25", "2")
			fetch(base+"&limit=2&q=thread-3", "3")
			fetch(base + "&limit=2&q=missing")
			child := fetch("archived=true&parent_id=0&limit=1&q=child", "1")
			fetch("archived=true&parent_id=0&limit=1&q=child&cursor="+url.QueryEscape(child.NextCursor), "3")
			fetch("", "4")
			fetch(base, "0", "1", "2", "3")
			if err := store.SetArchived("0", false); err != nil {
				t.Fatal(err)
			}
			fetch(base+"&limit=2&cursor="+url.QueryEscape(second.NextCursor), "2")
		})
	}
}

func TestListRejectsInvalidPagination(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewListHandler(store)
	for _, query := range []string{"limit=-1", "limit=0", "limit=201", "limit=oops", "limit=2&cursor=bad", "limit=2&cursor=1:", "limit=2&cursor=bad:id", "cursor=1:id"} {
		t.Run(query, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/sessions?"+query, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestListSessionsUpdatedSince(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.CreateSession(storage.CreateSession{Slug: "old", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	cutoff := time.Now().UTC()
	time.Sleep(5 * time.Millisecond)
	fresh, err := store.CreateSession(storage.CreateSession{Slug: "fresh", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewListHandler(store)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions?updated_since="+cutoff.Format(time.RFC3339Nano), nil))
	if res.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", res.Code, res.Body.String())
	}
	var listed struct {
		Sessions []storage.Session `json:"sessions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].ID != fresh.ID {
		t.Fatalf("expected only fresh session %s, got %#v (old=%s)", fresh.ID, listed.Sessions, old.ID)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions?updated_since=not-a-time", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for malformed updated_since, got %d", res.Code)
	}
}
