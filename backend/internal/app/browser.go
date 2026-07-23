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
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"go.uber.org/fx"
)

type BrowserSettingsHandler struct {
	http.Handler
}

func NewBrowserBackend(layout runtimefiles.Layout, store *sqlitestore.Store) *browsercontrol.ExtensionBridge {
	return browsercontrol.NewExtensionBridge(browsercontrol.NewLocalBackend(filepath.Join(layout.Root, "browser")), func() bool {
		settings, err := jazsettings.LoadBrowserSettings(store)
		return err != nil || jazsettings.BrowserUsesExtension(settings)
	})
}

func NewBrowserSettingsHandler(store *sqlitestore.Store, jaz *jaztools.Service, mcp *mcpruntime.Manager, backend *browsercontrol.ExtensionBridge) *BrowserSettingsHandler {
	return &BrowserSettingsHandler{Handler: browserapi.NewSettingsHandler(store, backend, func() {
		jaz.Sync()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mcp.Refresh(ctx)
		}()
	})}
}

func ConfigureBrowserTools(jaz *jaztools.Service, store *sqlitestore.Store, backend *browsercontrol.ExtensionBridge) {
	jaz.SetBrowser(store, backend)
}

func CloseBrowserBackend(lc fx.Lifecycle, backend *browsercontrol.ExtensionBridge) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return backend.Close()
		},
	})
}
