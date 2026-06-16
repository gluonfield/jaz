package app

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

type fakeUsageStore struct{}

func (fakeUsageStore) UsageEventsSince(time.Time) ([]storage.UsageEvent, error) {
	return nil, nil
}

func TestUsageModuleProvidesRoute(t *testing.T) {
	var routes server.Routes
	app := fx.New(
		fx.NopLogger,
		fx.Provide(func() storage.UsageEventStore { return fakeUsageStore{} }),
		UsageModule(),
		fx.Populate(&routes),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 ||
		routes[0].Pattern != "GET /v1/usage/daily" ||
		routes[0].Handler == nil ||
		routes[1].Pattern != "GET /v1/usage/models" ||
		routes[1].Handler == nil {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestUsageModuleWiresWithNewStore(t *testing.T) {
	var routes server.Routes
	var store *sqlitestore.Store
	app := fx.New(
		fx.NopLogger,
		fx.Supply(runtimefiles.New(t.TempDir()), acp.AgentCatalog{}),
		fx.Provide(NewStore),
		UsageModule(),
		fx.Populate(&routes, &store),
	)
	if err := app.Err(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if len(routes) != 2 || routes[0].Handler == nil || routes[1].Handler == nil {
		t.Fatalf("routes = %#v", routes)
	}
}
