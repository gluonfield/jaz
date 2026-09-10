package app

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/internal/browsercontrol"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

type BrowserSettingsHandler struct {
	http.Handler
}

func NewBrowserBackend(layout runtimefiles.Layout, store *sqlitestore.Store) *browsercontrol.ConfiguredBackend {
	return browsercontrol.NewConfiguredBackend(filepath.Join(layout.Root, "browser"), store)
}

func NewBrowserSettingsHandler(store *sqlitestore.Store, jaz *jaztools.Service, mcp *mcpruntime.Manager, backend *browsercontrol.ConfiguredBackend) *BrowserSettingsHandler {
	return &BrowserSettingsHandler{Handler: browserapi.NewSettingsHandler(store, backend, func() {
		jaz.Sync()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mcp.Refresh(ctx)
		}()
	})}
}

func ConfigureBrowserTools(jaz *jaztools.Service, store *sqlitestore.Store, backend *browsercontrol.ConfiguredBackend) {
	jaz.SetBrowser(store, backend)
}

func CloseBrowserBackend(lc fx.Lifecycle, backend *browsercontrol.ConfiguredBackend) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return backend.Close()
		},
	})
}
