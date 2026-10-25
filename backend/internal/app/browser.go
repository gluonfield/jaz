package app

import (
	"context"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/browsercontrol"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	"github.com/wins/jaz/backend/internal/jaztools"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

type BrowserSettingsHandler struct {
	http.Handler
}

func NewBrowserSettingsHandler(store *sqlitestore.Store, jaz *jaztools.Service, mcp *mcpruntime.Manager) *BrowserSettingsHandler {
	return &BrowserSettingsHandler{Handler: browserapi.NewSettingsHandler(store, func() {
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
