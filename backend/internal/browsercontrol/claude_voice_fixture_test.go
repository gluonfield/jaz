//go:build browserintegration

package browsercontrol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func mountClaudeVoiceFixture(mux *http.ServeMux) {
	var mu sync.Mutex
	active := 0
	opened := 0
	results := 0
	delayCreate := false
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><title>Voice provider fixture</title>"))
	})
	mux.HandleFunc("/fixture/claude", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost {
			delayCreate = true
		}
		json.NewEncoder(w).Encode(map[string]int{"active": active, "opened": opened, "results": results})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("voice-fixture-session")
		if err != nil || cookie.Value != "signed-in" {
			http.Error(w, "sign in", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/organizations":
			w.Write([]byte(`[{"uuid":"fixture-org"}]`))
		case r.URL.Path == "/api/organizations/fixture-org/model_selector/voice":
			w.Write([]byte(`{"model_selector_config":[{"id":"voice","models":[{"id":"fixture-sonnet","name":"Sonnet"},{"id":"retired","name":"Opus","disabled":true},{"id":"fixture-opus","name":"Opus"}]}]}`))
		case r.URL.Path == "/api/organizations/fixture-org/chat_conversations":
			var input struct {
				UUID      string `json:"uuid"`
				Temporary bool   `json:"is_temporary"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.UUID == "" || !input.Temporary {
				http.Error(w, "invalid conversation", http.StatusBadRequest)
				return
			}
			mu.Lock()
			delay := delayCreate
			mu.Unlock()
			if delay {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(300 * time.Millisecond):
				}
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"uuid": input.UUID})
		case strings.HasSuffix(r.URL.Path, "/tool_result"):
			var input struct {
				ID      string `json:"tool_use_id"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID != "tool-1" || len(input.Content) != 1 || input.Content[0].Text != "Agent accepted" {
				http.Error(w, "invalid result", http.StatusBadRequest)
				return
			}
			mu.Lock()
			results++
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/api/ws/voice/webrtc/organizations/fixture-org/chat_conversations/"):
			if r.URL.Query().Get("model") != "fixture-opus" || !strings.Contains(r.URL.Query().Get("capabilities"), "client_tools") {
				http.Error(w, "invalid voice selection", http.StatusBadRequest)
				return
			}
			connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			mu.Lock()
			active++
			opened++
			mu.Unlock()
			defer func() {
				connection.Close()
				mu.Lock()
				active--
				mu.Unlock()
			}()
			connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"session_server_initialized","data":{"session_id":"fixture-session","protocol":"2.1","media":"webrtc"}}`))
			for {
				_, data, err := connection.ReadMessage()
				if err != nil {
					return
				}
				var event struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &event) != nil {
					return
				}
				switch event.Type {
				case "user_message":
					connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"message_complete","data":{"sender":"human","content":[{"type":"text","text":"<untrusted_external_data source=\"client_observation\">host context</untrusted_external_data>"}]}}`))
				case "media_offer":
					connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"media_answer","data":{"sdp":"v=0\r\nfixture-answer"}}`))
				case "interrupt":
					connection.WriteMessage(websocket.TextMessage, []byte(`{"type":"server_interrupt"}`))
				case "close_stream":
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	})
}
