//go:build browserintegration

package browsercontrol_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	browserapi "github.com/wins/jaz/backend/internal/httpapi/browser"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDesktopElectron(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := settings.SaveBrowserSettings(store, settings.BrowserSettings{Enabled: true, Mode: settings.BrowserModeDesktop}); err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateSession(storage.CreateSession{Slug: "browser-fixture", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	backend := browsercontrol.NewConfiguredBackend(t.TempDir(), store)
	defer backend.Close()
	handler := browserapi.DesktopHandler{Backend: backend.Desktop, Store: store}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "browsercontrol", Version: "test"}, nil)
	browsercontrol.AddMCPTools(mcpServer, backend)
	mux := http.NewServeMux()
	mux.Handle("GET /v1/sessions/{session}/browser", handler)
	mux.Handle("POST /v1/sessions/{session}/browser", handler)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, nil))
	var client *mcp.ClientSession
	mux.HandleFunc("/exercise", func(w http.ResponseWriter, r *http.Request) {
		code := fmt.Sprintf(`await tab.goto(%q)
nodeRepl.write('🌏'.repeat(15000))
const state = await tab.find('Run this step')
const button = state.elements.find(element => element.role === 'button')
const cdp = tab.cdp
const response = await cdp.send('Runtime.evaluate', {expression:'document.title', returnByValue:true})
if (response.result.value !== 'Jaz browser control') {
  throw new Error('CDP response did not reach the MCP script')
}
await tab.scroll('down', 0, button.ref)
await tab.click(button.ref)
await tab.getState()
await tab.getScreenshot()`, r.URL.Query().Get("url"))
		result, err := client.CallTool(r.Context(), &mcp.CallToolParams{Name: browsercontrol.ToolScript, Arguments: map[string]any{"code": code}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var text strings.Builder
		var screenshot bool
		for _, content := range result.Content {
			switch content := content.(type) {
			case *mcp.TextContent:
				text.WriteString(content.Text)
			case *mcp.ImageContent:
				screenshot = len(content.Data) > 100 && content.MIMEType == "image/png"
			}
		}
		if result.IsError || !strings.Contains(text.String(), "Step completed with a trusted browser click") || !screenshot {
			http.Error(w, fmt.Sprintf("MCP result: error=%v screenshot=%v text=%s", result.IsError, screenshot, text.String()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()
	client, err = mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: &http.Client{Transport: sessionTransport{session: thread.ID}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, os.Getenv("JAZ_ELECTRON_BINARY"), filepath.Join(os.Getenv("JAZ_BROWSER_SMOKE_DIR"), "main.js"))
	command.Env = append(os.Environ(), "JAZ_BROWSER_SMOKE_BACKEND="+server.URL)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `{"ok":true,"checks":[`) {
		t.Fatal("Electron exited without completing the browser checks")
	}
}

type sessionTransport struct {
	session string
}

func (t sessionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set(mcpsession.HeaderName, t.session)
	return http.DefaultTransport.RoundTrip(request)
}
