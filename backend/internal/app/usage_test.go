package app

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
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
	if len(routes) != 1 || routes[0].Pattern != "GET /v1/usage/daily" || routes[0].Handler == nil {
		t.Fatalf("routes = %#v", routes)
	}
}
