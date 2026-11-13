package canvas

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/canvas"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func newTestServer(t *testing.T) (*http.ServeMux, string, string, *canvas.Service) {
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := func(surface string) string {
		created, err := store.CreateSession(storage.CreateSession{Title: surface, RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, ArtifactSurface: surface}})
		if err != nil {
			t.Fatal(err)
		}
		return created.ID
	}
	service := canvas.NewService(runtimefiles.New(t.TempDir()))
	handler := NewHandler(store, service)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/sessions/{session}/canvas", handler.Get)
	mux.HandleFunc("PATCH /v1/sessions/{session}/canvas", handler.Patch)
	mux.HandleFunc("PUT /v1/sessions/{session}/canvas/layout", handler.PutLayout)
	mux.HandleFunc("POST /v1/sessions/{session}/canvas/files", handler.PostFile)
	return mux, session("canvas"), session("chat"), service
}

func serve(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(method, path, strings.NewReader(body)))
	return res
}

func TestCanvasRoundTripAndUnchangedPolls(t *testing.T) {
	mux, board, _, _ := newTestServer(t)
	path := "/v1/sessions/" + board + "/canvas"

	res := serve(mux, http.MethodPatch, path, `{"page":"page-1","nodes":[{"id":"note","type":"text","text":"Hi","x":0,"y":0,"width":200,"height":200}]}`)
	if res.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", res.Code, res.Body.String())
	}
	res = serve(mux, http.MethodGet, path, "")
	var view canvas.View
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil || res.Code != http.StatusOK {
		t.Fatalf("get status = %d, err = %v", res.Code, err)
	}
	if len(view.Pages) != 1 || view.Pages[0].Name != "Page 1" || len(view.Pages[0].Nodes) != 1 || view.Pages[0].Nodes[0]["text"] != "Hi" {
		t.Fatalf("pages = %#v", view.Pages)
	}
	if res := serve(mux, http.MethodGet, path+"?version="+view.Version, ""); res.Code != http.StatusNoContent {
		t.Fatalf("unchanged poll status = %d, want 204", res.Code)
	}
}

func TestCanvasIsOnlyServedForCanvasThreads(t *testing.T) {
	mux, _, chat, _ := newTestServer(t)
	if res := serve(mux, http.MethodGet, "/v1/sessions/"+chat+"/canvas", ""); res.Code != http.StatusNotFound {
		t.Fatalf("chat thread status = %d, want 404", res.Code)
	}
	if res := serve(mux, http.MethodPatch, "/v1/sessions/"+chat+"/canvas", `{"remove":["x"]}`); res.Code != http.StatusNotFound {
		t.Fatalf("chat thread patch status = %d, want 404", res.Code)
	}
}

func TestCanvasPageCreationRenameAndInvalidTargets(t *testing.T) {
	mux, board, _, _ := newTestServer(t)
	path := "/v1/sessions/" + board + "/canvas"
	for _, body := range []string{
		`{"nodes":[{"id":"ambiguous"}]}`,
		`{"page":"unknown","nodes":[{"id":"misplaced"}]}`,
		`{"page":"page-1","create":true}`,
		`{"page":"page-1","delete":true}`,
		`{"page":"page-1","name":"  "}`,
		`{"page":" padded","create":true}`,
		`{"page":"padded ","create":true}`,
	} {
		if res := serve(mux, http.MethodPatch, path, body); res.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid page target status = %d, body = %s", res.Code, res.Body.String())
		}
	}
	for _, body := range []string{
		`{"page":"page-1","nodes":[{"id":"same","text":"First"}]}`,
		`{"page":"cost plan","create":true,"nodes":[{"id":"same","text":"Second"}]}`,
		`{"page":"cost plan","name":"  Forecast  "}`,
	} {
		if res := serve(mux, http.MethodPatch, path, body); res.Code != http.StatusOK {
			t.Fatalf("page patch status = %d, body = %s", res.Code, res.Body.String())
		}
	}
	res := serve(mux, http.MethodGet, path, "")
	var view canvas.View
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil || res.Code != http.StatusOK {
		t.Fatalf("get status = %d, err = %v", res.Code, err)
	}
	if len(view.Pages) != 2 || view.Pages[0].Nodes[0]["text"] != "First" || view.Pages[1].ID != "cost plan" || view.Pages[1].Name != "Forecast" || view.Pages[1].Nodes[0]["text"] != "Second" {
		t.Fatalf("pages = %#v", view.Pages)
	}
	if res := serve(mux, http.MethodPatch, path, `{"page":"cost plan","delete":true}`); res.Code != http.StatusOK {
		t.Fatalf("page deletion status = %d, body = %s", res.Code, res.Body.String())
	}
	res = serve(mux, http.MethodGet, path, "")
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil || res.Code != http.StatusOK || len(view.Pages) != 1 || view.Pages[0].Nodes[0]["text"] != "First" {
		t.Fatalf("deletion response = %s, err = %v", res.Body.String(), err)
	}
}

func TestLegacyCanvasVersionCannotReuseFlatResponse(t *testing.T) {
	mux, board, _, service := newTestServer(t)
	legacy := []byte(`{"nodes":[],"edges":[]}`)
	if err := os.MkdirAll(service.Dir(board), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service.Dir(board), canvas.FileName), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	oldHash := sha256.Sum256(legacy)
	oldVersion := hex.EncodeToString(oldHash[:])[:24]
	res := serve(mux, http.MethodGet, "/v1/sessions/"+board+"/canvas?version="+oldVersion, "")
	var view canvas.View
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil || res.Code != http.StatusOK || len(view.Pages) != 1 {
		t.Fatalf("flat cache reused for paged response: status=%d body=%s err=%v", res.Code, res.Body.String(), err)
	}
}

func TestLayoutIsSavedForCanvasPagesOnly(t *testing.T) {
	mux, board, chat, service := newTestServer(t)
	body := `{"page":"page-1","items":[{"id":"a","x":0,"y":0,"width":200,"height":88}],"issues":["` + "`a` overlaps `b`" + `"]}`

	if res := serve(mux, http.MethodPut, "/v1/sessions/"+board+"/canvas/layout", body); res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(service.Dir(board), canvas.LayoutFileName))
	if err != nil || !strings.Contains(string(raw), `{"id":"a","x":0,"y":0,"width":200,"height":88}`) {
		t.Fatalf("layout = %s, err = %v", raw, err)
	}
	if res := serve(mux, http.MethodPut, "/v1/sessions/"+board+"/canvas/layout", strings.Replace(body, "page-1", "gone", 1)); res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown page status = %d", res.Code)
	}
	if res := serve(mux, http.MethodPut, "/v1/sessions/"+chat+"/canvas/layout", body); res.Code != http.StatusNotFound {
		t.Fatalf("chat thread status = %d", res.Code)
	}
}

func TestUploadedFilesLandInTheCanvasFolder(t *testing.T) {
	mux, board, chat, service := newTestServer(t)
	upload := func(session, field, name, content string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte(content))
		form.Close()
		req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session+"/canvas/files", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}

	res := upload(board, "file", "site photo.png", "pixels")
	if res.Code != http.StatusOK || strings.TrimSpace(res.Body.String()) != `{"file":"uploads/site-photo.png"}` {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(service.Dir(board), "uploads", "site-photo.png"))
	if err != nil || string(saved) != "pixels" {
		t.Fatalf("saved = %q, err = %v", saved, err)
	}
	if res := upload(board, "other", "a.png", "x"); res.Code != http.StatusBadRequest {
		t.Fatalf("missing file field status = %d", res.Code)
	}
	if res := upload(chat, "file", "a.png", "x"); res.Code != http.StatusNotFound {
		t.Fatalf("chat thread status = %d", res.Code)
	}
}
