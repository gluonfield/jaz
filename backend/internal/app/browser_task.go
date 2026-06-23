package app

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsertask"
	"github.com/wins/jaz/backend/internal/browserworker"
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

func NewBrowserTaskService(store *sqlitestore.Store, manager *acp.Manager, catalog acp.AgentCatalog) *browsertask.Service {
	return browsertask.New(store, manager, catalog)
}

func NewBrowserWorkerBackend(layout runtimefiles.Layout) *browserworker.ExtensionBridge {
	return browserworker.NewExtensionBridge(browserworker.NewLocalBackend(filepath.Join(layout.Root, "browser")))
}

func NewBrowserSettingsHandler(store *sqlitestore.Store, catalog acp.AgentCatalog, jaz *jaztools.Service, mcp *mcpruntime.Manager) *BrowserSettingsHandler {
	return &BrowserSettingsHandler{Handler: browserapi.NewSettingsHandler(store, catalog, func() {
		jaz.Sync()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			mcp.Refresh(ctx)
		}()
	})}
}

func ConfigureBrowserTaskTools(jaz *jaztools.Service, browser *browsertask.Service, backend *browserworker.ExtensionBridge) {
	jaz.SetBrowser(browser, backend)
}

func CloseBrowserWorkerBackend(lc fx.Lifecycle, backend *browserworker.ExtensionBridge) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			return backend.Close()
		},
	})
}
