package bots

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	botcore "github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotFilesSaveOnlyOverTheVersionRead(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard)
	manager := acp.NewManager(store, acp.Config{Root: root, Agents: map[string]acp.AgentConfig{"fixture": {Command: "unused"}}}, logger)
	t.Cleanup(func() {
		manager.Close()
		_ = store.Close()
	})
	service := botcore.NewService(store, filepath.Join(root, "bots"), manager, nil, nil, loops.NewService(store, nil, logger), sessionevents.New(), logger)
	bot, err := service.Create(t.Context(), botcore.CreateBot{Name: "Research"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, sessionevents.New())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/bots/{bot}/files", handler.Files)
	mux.HandleFunc("PUT /v1/bots/{bot}/files/{name}", handler.SaveFile)
	call := func(method, path, body string, into any) int {
		t.Helper()
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
		if into != nil {
			if err := json.Unmarshal(recorder.Body.Bytes(), into); err != nil {
				t.Fatalf("%s %s: %v in %s", method, path, err, recorder.Body)
			}
		}
		return recorder.Code
	}
	files := "/v1/bots/" + bot.ID + "/files"

	var listed struct{ Files []botcore.File }
	if code := call("GET", files, "", &listed); code != http.StatusOK || len(listed.Files) != 2 || listed.Files[1].Name != "MEMORY.md" || listed.Files[1].Version != "" {
		t.Fatalf("GET files = %d %+v", code, listed)
	}
	var saved botcore.File
	if code := call("PUT", files+"/MEMORY.md", `{"content":"Biscuit is a dog.\n","version":""}`, &saved); code != http.StatusOK || saved.Content != "Biscuit is a dog.\n" || saved.Version == "" {
		t.Fatalf("creating MEMORY.md = %d %+v", code, saved)
	}
	var current botcore.File
	if code := call("PUT", files+"/MEMORY.md", `{"content":"lost\n","version":""}`, &current); code != http.StatusConflict || current.Content != "Biscuit is a dog.\n" || current.Version != saved.Version {
		t.Fatalf("saving over a stale version = %d %+v, want 409 with the file as it is", code, current)
	}
	if code := call("PUT", files+"/notes.md", `{"content":"x"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("saving a file that is not an identity file = %d, want 400", code)
	}
	if code := call("GET", "/v1/bots/missing/files", "", nil); code != http.StatusNotFound {
		t.Fatalf("files of a missing bot = %d, want 404", code)
	}
	// The user's edit is recorded in the bot's thread.
	events, err := store.LoadBotActivity(bot.ID, time.Unix(0, 0))
	if err != nil || len(events) != 1 || events[0].BotActivity.Kind != "file_edited" || events[0].BotActivity.Label != "MEMORY.md" {
		t.Fatalf("events after the user's edit = %+v, %v", events, err)
	}
}
