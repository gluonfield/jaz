package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

func TestServeFxGraphValidates(t *testing.T) {
	if err := fx.ValidateApp(serveOptions(nil)...); err != nil {
		t.Fatal(err)
	}
}

func TestServeStartupFailureDoesNotLeaveDeclarationsOrListener(t *testing.T) {
	for _, occupied := range []bool{true, false} {
		t.Run(fmt.Sprint("occupied=", occupied), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			addr := listener.Addr().String()
			cfg := app.Config{
				Root: t.TempDir(), Skills: app.SkillsConfig{DisableSync: true},
				ACP:  acp.Config{Agents: map[string]acp.AgentConfig{"fixture": {Command: "unused"}}},
				Bots: []bots.Config{{Name: "Research", Agent: "fixture"}},
			}
			if !occupied {
				_ = listener.Close()
				cfg.Bots[0].Prompt = "Research."
				cfg.Bots[0].Schedules = []bots.ScheduleConfig{{Name: "Morning", Schedule: loops.Schedule{Expr: "invalid"}}}
			}
			var store *sqlitestore.Store
			options := append(serveOptions([]string{"--addr", addr, "--public-url", "http://" + addr}),
				fx.Replace(cfg, log.New(io.Discard)), fx.Populate(&store), fx.NopLogger)
			application := fx.New(options...)
			if err := application.Err(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			err = application.Start(t.Context())
			if occupied {
				var networkError *net.OpError
				if !errors.As(err, &networkError) || networkError.Op != "listen" {
					t.Fatalf("startup error = %v, want listen failure", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "invalid cron expression") {
					t.Fatalf("startup error = %v, want invalid schedule", err)
				}
				rebound, err := net.Listen("tcp", addr)
				if err != nil {
					t.Fatalf("failed startup left the listener open: %v", err)
				}
				_ = rebound.Close()
			}
			records, err := store.ListBots()
			if err != nil || len(records) != 0 {
				t.Fatalf("failed startup declared bots: %+v, %v", records, err)
			}
		})
	}
}

func TestParseServeConfigIncludesPreviewURLTemplate(t *testing.T) {
	cfg, err := parseServeConfig(serveArgs{Args: []string{"--preview-url-template", " https://{id}.preview.example.test "}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PreviewURLTemplate != "https://{id}.preview.example.test" {
		t.Fatalf("preview URL template = %q", cfg.PreviewURLTemplate)
	}
}
