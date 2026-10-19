//go:build browserintegration

package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	botapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBotListElectron(t *testing.T) {
	if suite := os.Getenv("JAZ_BROWSER_SMOKE_SUITE"); suite != "" && suite != "bots-live" {
		return
	}
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	think := filepath.Join(root, "thinking")
	release := filepath.Join(root, "release")
	manager := newFakeAgentManager(t, store, root, map[string]string{
		"JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd",
		"JAZ_FAKE_ACP_THINK_HOLD":     think,
		"JAZ_FAKE_ACP_RELEASE_FILE":   release,
	})
	defer manager.Close()
	bus := sessionevents.New()
	manager.Events = bus
	logger := log.New(io.Discard)
	serverCore := &server.Server{Store: store, ACP: manager, Events: bus}
	manager.TurnFinished = serverCore.HandleACPTurnFinished
	service := bots.NewService(store, filepath.Join(root, "bots"), manager, serverCore, serverCore, loops.NewService(store, nil, logger), bus, logger)
	bot, err := service.Create(t.Context(), bots.CreateBot{Name: "Live Bot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Pin([]string{bot.ID}); err != nil {
		t.Fatal(err)
	}
	worker, err := manager.Spawn(t.Context(), acp.SpawnRequest{Slug: "worker", ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	handler := botapi.NewHandler(service, bus)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("GET /v1/bots", handler.List)
	mux.HandleFunc("GET /v1/bots/events", handler.Events)
	mux.HandleFunc("POST /exercise/{step}", func(w http.ResponseWriter, r *http.Request) {
		id := bot.ID
		if r.URL.Query().Get("worker") != "" {
			id = worker.SessionID
		}
		var err error
		switch r.PathValue("step") {
		case "start":
			_ = os.Remove(release)
			err = os.WriteFile(think, nil, 0600)
			if err == nil {
				_, err = manager.Send(r.Context(), acp.SendRequest{Session: id, Message: "until released", ParentVisible: true, Completion: acp.CompletionInline})
			}
		case "work":
			err = os.Remove(think)
		case "finish":
			err = os.WriteFile(release, nil, 0600)
		case "short":
			_, err = manager.Send(r.Context(), acp.SendRequest{Session: id, Message: "answer in the final reply", Completion: acp.CompletionInline})
		case "ask":
			_, err = manager.Send(r.Context(), acp.SendRequest{Session: id, Message: "ask then block", ParentVisible: true, Completion: acp.CompletionInline})
		case "cancel":
			_, err = manager.Cancel(r.Context(), id)
		case "status":
			var current bots.Bot
			current, err = service.Load(bot.ID)
			if err == nil {
				_ = json.NewEncoder(w).Encode(current)
				return
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": bot.ID})
	})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, os.Getenv("JAZ_ELECTRON_BINARY"), filepath.Join(os.Getenv("JAZ_BROWSER_SMOKE_DIR"), "main.js"))
	command.Env = append(os.Environ(), "JAZ_BROWSER_SMOKE_BACKEND="+httpServer.URL, "JAZ_BROWSER_SMOKE_SUITE=bots-live", "JAZ_BROWSER_SMOKE_TIMEOUT_MS=110000")
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `{"ok":true,"checks":[`) {
		t.Fatal("Electron exited without completing the live bot checks")
	}
}
