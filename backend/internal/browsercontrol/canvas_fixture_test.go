//go:build browserintegration

package browsercontrol_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/wins/jaz/backend/internal/canvas"
	canvasapi "github.com/wins/jaz/backend/internal/httpapi/canvas"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func mountCanvasFixture(t *testing.T, mux *http.ServeMux, store *sqlitestore.Store) {
	t.Helper()
	session, err := store.CreateSession(storage.CreateSession{Slug: "canvas-pages", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, ArtifactSurface: "canvas"}})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewService(runtimefiles.New(t.TempDir()))
	if err := os.MkdirAll(service.Dir(session.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(service.Dir(session.ID), canvas.FileName)
	if err := os.WriteFile(path, []byte(`{"nodes":[{"id":"same","type":"text","text":"Legacy page","x":0,"y":0,"width":240,"height":160}],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := canvasapi.NewHandler(store, service)
	mux.HandleFunc("GET /v1/sessions/{session}/canvas", handler.Get)
	mux.HandleFunc("PATCH /v1/sessions/{session}/canvas", handler.Patch)
	type contents struct {
		Content string `json:"content"`
	}
	mux.HandleFunc("GET /exercise-canvas", func(w http.ResponseWriter, r *http.Request) {
		raw, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(contents{Content: string(raw)})
	})
	mux.HandleFunc("POST /exercise-canvas", func(w http.ResponseWriter, r *http.Request) {
		var request contents
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(path, []byte(request.Content), 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
