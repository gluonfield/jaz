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
	mux.HandleFunc("/exercise-accessibility", func(w http.ResponseWriter, r *http.Request) {
		code := fmt.Sprintf(`await tab.goto(%q)
function axIndex(tree, role, name) {
  const match = tree.match(new RegExp('^\\s*(\\d+) ' + role + ' "' + name + '"', 'm'))
  if (!match) {
    throw new Error('Missing AX target: ' + role + ' ' + name + '\n' + tree)
  }
  return Number(match[1])
}
const firstAX = await tab.getAXState()
if (firstAX.includes('Hidden exclusion sentinel') || firstAX.includes('Hidden frame action') || firstAX.includes('secret-must-not-appear')) {
  throw new Error('Hidden or password content leaked into AX observations')
}
const fieldAX = axIndex(firstAX, 'textbox', 'Computed field label')
const updateAX = axIndex(firstAX, 'button', 'Update status')
const shadowAX = axIndex(firstAX, 'button', 'Closed shadow action')
const sameAX = axIndex(firstAX, 'button', 'Same-site action')
const crossAX = axIndex(firstAX, 'button', 'Cross-site action')
const crossFieldAX = axIndex(firstAX, 'textbox', 'Cross-site action field')
const nestedSameAX = axIndex(firstAX, 'button', 'Nested Same-site action')
const nestedCrossAX = axIndex(firstAX, 'button', 'Nested Cross-site action')
const unchangedAX = await tab.getAXState()
if (!unchangedAX.includes('Accessibility tree unchanged')) {
  throw new Error('Identical AX observations did not produce an unchanged result: ' + unchangedAX)
}
await tab.setValue(fieldAX, 'AX field value')
await tab.click(updateAX)
const changedAX = await tab.getAXState()
if (!changedAX.includes('Verified AX action') || !changedAX.includes('Added:')) {
  throw new Error('AX change diff missed the updated status: ' + changedAX)
}
const fullAX = await tab.getAXState({disableDiffing:true})
if (axIndex(fullAX, 'button', 'Update status') !== updateAX || !fullAX.includes('AX field value')) {
  throw new Error('AX indices changed identity or form value was absent')
}
await tab.click(shadowAX)
await tab.click(axIndex(firstAX, 'button', 'Wrapped action'))
await tab.cdp.send('Runtime.evaluate', {expression:[
  'const range = document.createRange()',
  'range.selectNodeContents(document.getElementById("offset"))',
  'const bounds = range.getBoundingClientRect()',
  'const textCover = document.createElement("div")',
  'textCover.id="text-cover"',
  'Object.assign(textCover.style, {position:"absolute",left:bounds.left+scrollX+"px",top:bounds.top+scrollY+"px",width:bounds.width+"px",height:bounds.height+"px",zIndex:"99999"})',
  'document.body.append(textCover)'
].join('\n')})
await tab.click(axIndex(firstAX, 'StaticText', 'Offset label'))
const offsetClick = await tab.cdp.send('Runtime.evaluate', {expression:'window.trustedOffset',returnByValue:true})
if (offsetClick.result.value !== true) {
  throw new Error('AX click moved away from the hit-tested point onto the covering element')
}
await tab.cdp.send('Runtime.evaluate', {expression:'document.getElementById("text-cover").remove()'})
await tab.cdp.send('Runtime.evaluate', {expression:[
  'const overlay = document.createElement("div")',
  'overlay.id="frame-cover"',
  'overlay.style="position:fixed;inset:0;z-index:99999;background:white"',
  'document.body.append(overlay)'
].join('\n')})
for (const coveredIndex of [shadowAX, sameAX, crossAX]) {
  let coverRejected = false
  try {
    await tab.hover(coveredIndex)
  } catch {
    coverRejected = true
  }
  if (!coverRejected) {
    throw new Error('Obscured AX target was accepted: ' + coveredIndex)
  }
}
await tab.cdp.send('Runtime.evaluate', {expression:'document.getElementById("frame-cover").remove()'})
await tab.click(sameAX)
await tab.click(crossAX)
await tab.click(nestedSameAX)
await tab.click(nestedCrossAX)
await tab.setValue(crossFieldAX, 'Frame value')
const frameAX = await tab.getAXState()
if (!frameAX.includes('Frame value')) {
  throw new Error('Cross-site form input did not update the AX tree')
}
const verification = await tab.cdp.send('Runtime.evaluate', {
  expression: '({root:!!window.trustedAX,shadow:!!window.trustedShadow,fragment:!!window.trustedFragment,same:!!window["Same-site action"],cross:!!window["Cross-site action"],nestedSame:!!window["Nested Same-site action"],nestedCross:!!window["Nested Cross-site action"]})',
  returnByValue: true
})
if (!Object.values(verification.result.value).every(value => value === true)) {
  throw new Error('AX actions did not produce trusted input on every target: ' + JSON.stringify(verification))
}
await tab.cdp.send('Runtime.evaluate', {expression:'document.documentElement.style.scrollBehavior = "smooth"'})
try {
  await tab.click(axIndex(firstAX, 'button', 'Smooth scrolling action'))
} catch (error) {
  throw new Error('Smooth scrolling target failed: ' + error.message)
}
const smoothClick = await tab.cdp.send('Runtime.evaluate', {expression:'window.trustedSmooth',returnByValue:true})
if (smoothClick.result.value !== true) {
  throw new Error('Page smooth scrolling moved the target while aiming its click')
}
await tab.cdp.send('Runtime.evaluate', {expression:'document.documentElement.style.scrollBehavior = "auto"'})
await tab.scroll('down', 0, axIndex(firstAX, 'RootWebArea', 'AX fixture'))
await tab.cdp.send('Runtime.evaluate', {expression:'document.querySelector("button").remove()'})
await tab.getAXState()
let removedRejectedAX = false
try {
  await tab.click(updateAX)
} catch {
  removedRejectedAX = true
}
if (!removedRejectedAX) {
  throw new Error('A removed AX target was reused')
}
await tab.getScreenshot()
const afterScreenshotAX = await tab.getAXState()
if (!afterScreenshotAX.includes('Accessibility fixture') || afterScreenshotAX.includes('Accessibility tree unchanged')) {
  throw new Error('Screenshot did not force the next full AX tree')
}
await tab.goto(%q)
let navigationRejectedAX = false
try {
  await tab.click(shadowAX)
} catch {
  navigationRejectedAX = true
}
if (!navigationRejectedAX) {
  throw new Error('AX index survived document replacement')
}
nodeRepl.write('Accessibility checks completed')`, r.URL.Query().Get("url"), r.URL.Query().Get("url")+"?new-document")
		result, err := client.CallTool(r.Context(), &mcp.CallToolParams{Name: browsercontrol.ToolScript, Arguments: map[string]any{"code": code}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var output strings.Builder
		for _, content := range result.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				output.WriteString(text.Text)
			}
		}
		if result.IsError || !strings.Contains(output.String(), "Accessibility checks completed") {
			http.Error(w, output.String(), http.StatusInternalServerError)
			return
		}
		observation, err := client.CallTool(r.Context(), &mcp.CallToolParams{Name: browsercontrol.ToolAXState, Arguments: map[string]any{"disable_diffing": true}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var observedText strings.Builder
		for _, content := range observation.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				observedText.WriteString(text.Text)
			}
		}
		observed := observedText.String()
		if observation.IsError || !strings.Contains(observed, "Closed shadow action") || !strings.Contains(observed, "Cross-site action") {
			http.Error(w, "Direct MCP AX observation did not expose the page: "+observed, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/exercise-codex", func(w http.ResponseWriter, r *http.Request) {
		if binary := os.Getenv("JAZ_BROWSER_CODEX_BINARY"); binary != "" {
			if err := checkCodexBrowser(r.Context(), binary, "http://"+r.Host+"/mcp", thread.ID, r.URL.Query().Get("url"), client); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
