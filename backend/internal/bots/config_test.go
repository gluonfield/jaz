package bots

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/promptmodule"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func newConfiguredBotService(t *testing.T, root string) (*Service, *sqlitestore.Store, *acp.Manager) {
	t.Helper()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	logger := log.New(io.Discard)
	manager := acp.NewManager(store, acp.Config{
		Root: root,
		Agents: map[string]acp.AgentConfig{
			"fixture": {Command: "unused", Model: "default-model"},
			"other":   {Command: "unused", Model: "other-default"},
		},
	}, logger)
	t.Cleanup(func() {
		manager.Close()
		_ = store.Close()
	})
	inst := instructions.NewLocalFSStore(root, instructions.WithBotHomeResolver(HomeResolver(store)))
	service := NewService(store, filepath.Join(root, "bots"), inst, manager, nil, nil,
		loops.NewService(store, nil, logger), sessionevents.New(), logger)
	return service, store, manager
}

func TestConfiguredBotsReconcileAcrossRestarts(t *testing.T) {
	root := t.TempDir()
	service, store, manager := newConfiguredBotService(t, root)
	config := []Config{{
		Name: "Research", Prompt: "Follow robotics research.", Agent: " FIXTURE ", Model: "chosen-model",
		Schedules: []ScheduleConfig{
			{Name: "Morning", Schedule: loops.Schedule{Expr: "0 9 * * 1-5", Timezone: "Europe/London"}},
			{Name: "Weekly", Prompt: "Write a weekly review.", Status: loops.StatusPaused, Schedule: loops.Schedule{Expr: "0 15 * * 5", Timezone: "UTC"}},
		},
	}, {Name: "Operations"}}
	if err := service.Declare(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	bots, err := service.List()
	if err != nil || len(bots) != 2 {
		t.Fatalf("bots = %+v, %v", bots, err)
	}
	var botID string
	for _, bot := range bots {
		if bot.Name == "Research" {
			botID = bot.ID
			if bot.Agent != "fixture" || bot.Model != "chosen-model" || bot.Prompt != config[0].Prompt {
				t.Fatalf("configured bot = %+v", bot)
			}
		} else if bot.Agent != "fixture" || bot.Model != "default-model" {
			t.Fatalf("bot defaults = %+v", bot)
		}
	}
	routines, err := service.routines.List()
	if err != nil || len(routines) != 2 {
		t.Fatalf("routines = %+v, %v", routines, err)
	}
	var morning loops.Loop
	for _, routine := range routines {
		if routine.BotID != botID {
			t.Fatalf("routine belongs to %q, want %q", routine.BotID, botID)
		}
		if routine.Name == "Morning" {
			morning = routine
			if routine.Prompt != config[0].Prompt || routine.Schedule.Timezone != "Europe/London" || routine.Status != loops.StatusActive || routine.NextRunAt.IsZero() {
				t.Fatalf("morning routine = %+v", routine)
			}
		} else if routine.Prompt != "Write a weekly review." || routine.Status != loops.StatusPaused {
			t.Fatalf("weekly routine = %+v", routine)
		}
	}
	morning.LastRunAt = time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	morning.LastRunID = store.NewRunID()
	morning.LastRunStatus = loops.RunStatusOK
	if err := store.SaveLoop(morning); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(botID, sessionevents.Event{
		Type: sessionevents.TypeRoomMessage, At: time.Now().UTC(),
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "user", Name: "You", Text: "Keep this conversation."},
	}); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	service, store, _ = newConfiguredBotService(t, root)
	config[0].Name = " research "
	if err := service.Declare(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	after, err := store.LoadLoop(morning.ID)
	if err != nil || !after.NextRunAt.Equal(morning.NextRunAt) || after.LastRunID != morning.LastRunID || !after.LastRunAt.Equal(morning.LastRunAt) || !after.UpdatedAt.Equal(morning.UpdatedAt) {
		t.Fatalf("routine changed on restart = %+v, %v", after, err)
	}
	config[0].Prompt = "Track industrial robotics."
	config[0].Agent = "other"
	config[0].Model = "new-model"
	config[0].Schedules[0].Schedule.Expr = "30 10 * * 1-5"
	config[0].Schedules[1].Status = loops.StatusActive
	if err := service.Declare(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	bot, err := service.Load(botID)
	if err != nil || bot.Agent != "other" || bot.Model != "new-model" || bot.Prompt != config[0].Prompt || bot.Preview != "Keep this conversation." {
		t.Fatalf("updated bot = %+v, %v", bot, err)
	}
	after, err = store.LoadLoop(morning.ID)
	if err != nil || after.Prompt != config[0].Prompt || after.Schedule.Expr != "30 10 * * 1-5" || after.LastRunID != morning.LastRunID || after.NextRunAt.Equal(morning.NextRunAt) {
		t.Fatalf("updated routine = %+v, %v", after, err)
	}
	if err := service.Declare(t.Context(), []Config{{Name: "Research", Prompt: config[0].Prompt}}); err != nil {
		t.Fatal(err)
	}
	if bot, err = service.Load(botID); err != nil || bot.Agent != "other" || bot.Model != "new-model" {
		t.Fatalf("omitted agent/model overwrote choices = %+v, %v", bot, err)
	}
	if bots, err = service.List(); err != nil || len(bots) != 2 {
		t.Fatalf("bots duplicated = %+v, %v", bots, err)
	}
	if routines, err = service.routines.List(); err != nil || len(routines) != 2 {
		t.Fatalf("routines duplicated or removed = %+v, %v", routines, err)
	}
	for _, routine := range routines {
		if routine.Name == "Weekly" && routine.Status != loops.StatusActive {
			t.Fatalf("weekly routine stayed paused: %+v", routine)
		}
	}
}

func TestConfiguredBotsRejectInvalidInputBeforeCreatingAnything(t *testing.T) {
	for _, test := range []struct {
		name   string
		config []Config
	}{
		{"missing bot name", []Config{{Prompt: "Research."}}},
		{"duplicate bot", []Config{{Name: "Research"}, {Name: " research "}}},
		{"Unicode duplicate bot", []Config{{Name: "S"}, {Name: "ſ"}}},
		{"missing schedule name", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{{Schedule: loops.Schedule{Expr: "0 9 * * *"}}}}}},
		{"duplicate schedule", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{
			{Name: "Morning", Schedule: loops.Schedule{Expr: "0 9 * * *"}},
			{Name: " morning ", Schedule: loops.Schedule{Expr: "0 10 * * *"}},
		}}}},
		{"Unicode duplicate schedule", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{
			{Name: "Σ", Schedule: loops.Schedule{Expr: "0 9 * * *"}},
			{Name: "ς", Schedule: loops.Schedule{Expr: "0 10 * * *"}},
		}}}},
		{"invalid cron", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{{Name: "Morning", Schedule: loops.Schedule{Expr: "bad"}}}}}},
		{"invalid timezone", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{{Name: "Morning", Schedule: loops.Schedule{Expr: "0 9 * * *", Timezone: "bad"}}}}}},
		{"missing prompt", []Config{{Name: "Research", Schedules: []ScheduleConfig{{Name: "Morning", Schedule: loops.Schedule{Expr: "0 9 * * *"}}}}}},
		{"invalid status", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{{Name: "Morning", Status: "bad", Schedule: loops.Schedule{Expr: "0 9 * * *"}}}}}},
		{"event without trigger", []Config{{Name: "Research", Prompt: "Research.", Schedules: []ScheduleConfig{{Name: "Morning", Schedule: loops.Schedule{Kind: loops.ScheduleEvent}}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, store, _ := newConfiguredBotService(t, t.TempDir())
			config := append([]Config{{Name: "Valid"}}, test.config...)
			if err := service.Declare(t.Context(), config); err == nil {
				t.Fatal("invalid config accepted")
			}
			sessions, err := store.ListSessions(storage.SessionFilter{IncludeSourced: true})
			if err != nil || len(sessions) != 0 {
				t.Fatalf("sessions created before validation = %+v, %v", sessions, err)
			}
		})
	}
}

func TestConfiguredPromptReachesOwnAndGroupChats(t *testing.T) {
	service, store, manager := newConfiguredBotService(t, t.TempDir())
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research", Prompt: "  Follow robotics research.  "})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := service.Create(t.Context(), CreateBot{Name: "Operations"})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.CreateGroup("Launch", []string{bot.ID, peer.ID})
	if err != nil {
		t.Fatal(err)
	}
	seat, err := manager.CreateSession(t.Context(), acp.SpawnRequest{Slug: "seat", SourceType: storage.SourceBotMember, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: bot.ID, ThreadID: seat.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{bot.ID, seat.ID} {
		thread, err := store.LoadSession(id)
		if err != nil {
			t.Fatal(err)
		}
		modules, err := Prompt(store, service.instructions, thread)
		if err != nil || len(modules) != 2 || modules[1] != "Follow robotics research." || !strings.Contains(modules[0], "send_message") {
			t.Fatalf("prompt for %s = %v, %v", id, modules, err)
		}
	}
}

func TestBotPromptIsPersistedBeforeModelResume(t *testing.T) {
	root := t.TempDir()
	service, store, _ := newConfiguredBotService(t, root)
	bot, err := service.Create(t.Context(), CreateBot{Name: "Research", Agent: "fixture", Prompt: "Old instructions."})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	thread.RuntimeRef.SessionID = "stored-provider-session"
	if err := store.SaveSession(thread); err != nil {
		t.Fatal(err)
	}
	var supplied promptmodule.Modules
	manager := acp.NewManager(store, acp.Config{
		Root: root, Agents: map[string]acp.AgentConfig{"fixture": {Command: filepath.Join(root, "missing-agent")}},
		ResumePrompt: func(session storage.Session) (promptmodule.Modules, error) {
			modules, err := Prompt(store, service.instructions, session)
			supplied = modules
			return modules, err
		},
	}, log.New(io.Discard))
	t.Cleanup(manager.Close)
	service.threads = manager
	if _, err := service.Update(t.Context(), bot.ID, UpdateBot{Prompt: new("New instructions."), Model: new("new-model")}); err == nil {
		t.Fatal("unavailable fixture executable accepted a live model change")
	}
	if len(supplied) != 2 || supplied[1] != "New instructions." {
		t.Fatalf("resuming the model saw stale bot instructions: %v", supplied)
	}
}

func TestConfiguredBotNamesRejectAmbiguousSavedRecords(t *testing.T) {
	for _, kind := range []string{"bots", "group", "routines"} {
		t.Run(kind, func(t *testing.T) {
			service, _, _ := newConfiguredBotService(t, t.TempDir())
			first, err := service.Create(t.Context(), CreateBot{Name: "Research"})
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.Create(t.Context(), CreateBot{Name: "Research"})
			if err != nil {
				t.Fatal(err)
			}
			config := []Config{{Name: "Research"}}
			if kind != "bots" {
				if _, err := service.Update(t.Context(), second.ID, UpdateBot{Name: new("Operations")}); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "group" {
				if _, err := service.CreateGroup("Research", []string{first.ID, second.ID}); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "routines" {
				for range 2 {
					if _, err := service.routines.Create(loops.CreateLoop{
						Name: "Morning", Prompt: "Research.", BotID: first.ID, Schedule: loops.Schedule{Expr: "0 9 * * *"},
					}); err != nil {
						t.Fatal(err)
					}
				}
				config[0].Schedules = []ScheduleConfig{{Name: "Morning", Prompt: "Research.", Schedule: loops.Schedule{Expr: "0 9 * * *"}}}
			}
			before, err := service.List()
			if err != nil {
				t.Fatal(err)
			}
			config = append([]Config{{Name: "New bot"}}, config...)
			if err := service.Declare(t.Context(), config); err == nil {
				t.Fatal("ambiguous name accepted")
			}
			after, err := service.List()
			if err != nil || len(after) != len(before) {
				t.Fatalf("ambiguous config created a bot: before %d, after %d, error %v", len(before), len(after), err)
			}
		})
	}
}
