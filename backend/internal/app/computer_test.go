package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/computercontrol"
	computerapi "github.com/wins/jaz/backend/internal/httpapi/computer"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestComputerRoutesAuthenticateAndRouteToDesktop(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := computercontrol.NewDesktopBackend()
	defer backend.Close()
	saved := 0
	handler := &ComputerSettingsHandler{Handler: computerapi.SettingsHandler{Store: store, OnSave: func() { saved++ }}}
	routes := NewRoutes(routeDeps{Store: store, Computer: backend, ComputerSettings: handler})
	httpServer := httptest.NewServer((&server.Server{Store: store, Routes: routes, AuthKey: "test-key"}).Handler())
	defer httpServer.Close()
	request := func(method, path, body, key string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, httpServer.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		return response
	}
	for _, path := range []string{"/v1/computer", "/v1/sessions/missing/computer"} {
		if response := request(http.MethodGet, path, "", ""); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s=%d", path, response.StatusCode)
		}
	}
	if response := request(http.MethodGet, "/v1/sessions/missing/computer", "", "test-key"); response.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled computer=%d", response.StatusCode)
	}
	if response := request(http.MethodPut, "/v1/computer", `{"enabled":true}`, "test-key"); response.StatusCode != http.StatusOK || saved != 1 {
		t.Fatalf("settings save=%d callbacks=%d", response.StatusCode, saved)
	}
	if response := request(http.MethodGet, "/v1/sessions/missing/computer", "", "test-key"); response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown session=%d", response.StatusCode)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "native-test", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/sessions/" + session.Slug + "/computer"
	peer, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Authorization": {"Bearer test-key"}})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	go func() {
		var command struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if peer.ReadJSON(&command) == nil {
			_ = peer.WriteJSON(map[string]any{"id": command.ID, "result": map[string]any{
				"status": "connected", "text": "client machine", "data": map[string]any{"platform": "darwin"},
			}})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for {
		output, err := backend.Call(ctx, computercontrol.ActionInput{Session: session.ID, Action: computercontrol.ActionStatus})
		if err != nil {
			t.Fatal(err)
		}
		if output.Status == "connected" {
			if output.Text != "client machine" || string(output.Data) != `{"platform":"darwin"}` {
				t.Fatalf("desktop response=%+v", output)
			}
			break
		}
		if ctx.Err() != nil {
			t.Fatal("slug connection never reached canonical session")
		}
		time.Sleep(time.Millisecond)
	}
	response := request(http.MethodGet, "/v1/computer", "", "test-key")
	var config settings.ComputerSettings
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil || !config.Enabled {
		t.Fatalf("persisted settings=%+v error=%v", config, err)
	}
}
