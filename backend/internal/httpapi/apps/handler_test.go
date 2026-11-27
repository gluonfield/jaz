package apps_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appscore "github.com/wins/jaz/backend/internal/apps"
	appsapi "github.com/wins/jaz/backend/internal/httpapi/apps"
	"github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestHTTPAppsDraftPublishTicketContentAndKV(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(root)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	svc := appscore.NewService(store, store, nil, nil, root, ws)
	h := appsapi.NewHandler(svc, "http://127.0.0.1:4242")

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/apps/drafts", h.CreateDraft)
	mux.HandleFunc("POST /v1/apps", h.Create)
	mux.HandleFunc("GET /v1/apps", h.List)
	mux.HandleFunc("POST /v1/apps/{id}/ticket", h.IssueTicket)
	mux.HandleFunc("PUT /v1/apps/{id}/kv/{key}", h.PutKV)
	mux.HandleFunc("GET /v1/apps/{id}/kv/{key}", h.GetKV)
	mux.HandleFunc("DELETE /v1/apps/{id}", h.Delete)
	mux.HandleFunc("/app-content/", h.ServeContent)

	draftReq := httptest.NewRequest(http.MethodPost, "/v1/apps/drafts", strings.NewReader(`{
		"title": "Calculator",
		"kind": "artifact",
		"entry": "index.html",
		"html": "<h1>Calc</h1>",
		"capabilities": ["storage:personal"]
	}`))
	draftRec := httptest.NewRecorder()
	mux.ServeHTTP(draftRec, draftReq)
	if draftRec.Code != http.StatusCreated {
		t.Fatalf("draft status=%d body=%s", draftRec.Code, draftRec.Body.String())
	}
	var draftResp struct {
		DraftID      string `json:"draft_id"`
		BundleSHA256 string `json:"bundle_sha256"`
	}
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draftResp); err != nil {
		t.Fatal(err)
	}

	pubBody, _ := json.Marshal(map[string]any{
		"draft_id":      draftResp.DraftID,
		"bundle_sha256": draftResp.BundleSHA256,
		"visibility":    "private",
	})
	pubRec := httptest.NewRecorder()
	mux.ServeHTTP(pubRec, httptest.NewRequest(http.MethodPost, "/v1/apps", bytes.NewReader(pubBody)))
	if pubRec.Code != http.StatusCreated {
		t.Fatalf("publish status=%d body=%s", pubRec.Code, pubRec.Body.String())
	}
	var pubResp struct {
		App struct {
			ID string `json:"id"`
		} `json:"app"`
	}
	if err := json.Unmarshal(pubRec.Body.Bytes(), &pubResp); err != nil {
		t.Fatal(err)
	}
	appID := pubResp.App.ID

	directRec := httptest.NewRecorder()
	mux.ServeHTTP(directRec, httptest.NewRequest(http.MethodPost, "/v1/apps", strings.NewReader(`{"title":"Notes","markdown":"# Notes"}`)))
	var direct struct {
		App struct {
			Kind       string `json:"kind"`
			Visibility string `json:"visibility"`
		} `json:"app"`
	}
	_ = json.Unmarshal(directRec.Body.Bytes(), &direct)
	if directRec.Code != http.StatusCreated || direct.App.Kind != "document" || direct.App.Visibility != "private" {
		t.Fatalf("direct publish status=%d body=%s", directRec.Code, directRec.Body.String())
	}
	elevatedRec := httptest.NewRecorder()
	mux.ServeHTTP(elevatedRec, httptest.NewRequest(http.MethodPost, "/v1/apps", strings.NewReader(`{"title":"Bot","html":"<p>hi</p>","capabilities":["tools:call"],"allowed_tools":["crm_search"]}`)))
	if elevatedRec.Code != http.StatusForbidden {
		t.Fatalf("direct publish of an elevated app must be refused, got %d body=%s", elevatedRec.Code, elevatedRec.Body.String())
	}

	// Ticket + /app-content sandbox CSP & Service-Worker denial.
	tRec := httptest.NewRecorder()
	mux.ServeHTTP(tRec, httptest.NewRequest(http.MethodPost, "/v1/apps/"+appID+"/ticket", nil))
	if tRec.Code != http.StatusOK {
		t.Fatalf("ticket status=%d body=%s", tRec.Code, tRec.Body.String())
	}
	var tResp struct {
		Ticket string `json:"ticket"`
	}
	_ = json.Unmarshal(tRec.Body.Bytes(), &tResp)

	swReq := httptest.NewRequest(http.MethodGet, "/app-content/"+appID+"/index.html?ticket="+tResp.Ticket, nil)
	swReq.Header.Set("Service-Worker", "script")
	swRec := httptest.NewRecorder()
	mux.ServeHTTP(swRec, swReq)
	if swRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for Service-Worker: script, got %d", swRec.Code)
	}

	cRec := httptest.NewRecorder()
	mux.ServeHTTP(cRec, httptest.NewRequest(http.MethodGet, "/app-content/"+appID+"/index.html?ticket="+tResp.Ticket, nil))
	if cRec.Code != http.StatusOK || cRec.Body.String() != "<h1>Calc</h1>" {
		t.Fatalf("content status=%d body=%q", cRec.Code, cRec.Body.String())
	}
	if !strings.Contains(cRec.Header().Get("Content-Security-Policy"), "sandbox allow-scripts allow-forms") {
		t.Fatalf("missing opaque sandbox CSP: %q", cRec.Header().Get("Content-Security-Policy"))
	}

	// KV write + conflict check.
	putReq := httptest.NewRequest(http.MethodPut, "/v1/apps/"+appID+"/kv/count", strings.NewReader(`{"value":"42"}`))
	putRec := httptest.NewRecorder()
	mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK || putRec.Header().Get("ETag") != `"rev-1"` {
		t.Fatalf("kv put status=%d etag=%q body=%s", putRec.Code, putRec.Header().Get("ETag"), putRec.Body.String())
	}

	staleReq := httptest.NewRequest(http.MethodPut, "/v1/apps/"+appID+"/kv/count", strings.NewReader(`{"value":"43"}`))
	staleReq.Header.Set("If-Match", `"rev-99"`)
	staleRec := httptest.NewRecorder()
	mux.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 on stale If-Match, got %d", staleRec.Code)
	}
}
