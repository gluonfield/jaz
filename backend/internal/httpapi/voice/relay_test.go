package voice_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wins/jaz/backend/internal/deviceauth"
	voiceapi "github.com/wins/jaz/backend/internal/httpapi/voice"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/voice/live"
)

type relayFixture struct {
	server  *httptest.Server
	devices *deviceauth.Service
	desktop deviceauth.RegisterResult
	phones  []deviceauth.RegisterResult
	session string
}

type hostFrame struct {
	ID     int64              `json:"id"`
	Method string             `json:"method"`
	Params live.ClaudeCommand `json:"params"`
	Wire   string             `json:"-"`
}

type phoneFrame struct {
	ID    string          `json:"id"`
	Type  string          `json:"type"`
	Data  string          `json:"data"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

func newRelayFixture(t *testing.T) *relayFixture {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	devices := deviceauth.New(store)
	register := func(kind string, seed byte) deviceauth.RegisterResult {
		t.Helper()
		raw := make([]byte, 32)
		raw[0] = seed
		sum := sha256.Sum256(raw)
		device, err := devices.RegisterApproved(deviceauth.Registration{
			Identity: deviceauth.DeviceIdentity{DeviceID: hex.EncodeToString(sum[:]), PublicKey: base64.RawURLEncoding.EncodeToString(raw)},
			Profile:  deviceauth.DeviceProfile{Name: "Voice fixture", Kind: kind},
		})
		if err != nil {
			t.Fatal(err)
		}
		return device
	}
	fixture := &relayFixture{devices: devices, desktop: register("desktop", 1), phones: []deviceauth.RegisterResult{register("mobile", 2), register("mobile", 3)}}
	session, err := store.CreateSession(storage.CreateSession{Title: "Voice fixture"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.session = session.ID
	service := live.NewService(store, func(string) (live.Credential, error) {
		return live.Credential{}, errors.New("fixture has no OpenAI credentials")
	})
	if _, err := service.SaveSettings(live.Settings{Agent: "claude", Provider: "claude", Voice: "buttery"}); err != nil {
		t.Fatal(err)
	}
	handler := voiceapi.NewRelayHandler(store, service, live.NewDesktopRelay())
	fixture.server = httptest.NewServer((&server.Server{Store: store, Devices: devices, AuthKey: "fixture-owner", Routes: server.Routes{
		{Pattern: "GET /v1/voice/desktop", Handler: http.HandlerFunc(handler.Desktop)},
		{Pattern: "GET /v1/voice/claude/status", Handler: http.HandlerFunc(handler.Status)},
		{Pattern: "GET /v1/sessions/{session}/voice/claude", Handler: http.HandlerFunc(handler.Call)},
	}}).Handler())
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *relayFixture) dial(t *testing.T, path, token string) *websocket.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+path, http.Header{"Authorization": {"Bearer " + token}})
		if err != nil {
			status := 0
			if response != nil {
				status = response.StatusCode
				response.Body.Close()
			}
			if status == http.StatusServiceUnavailable && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("dial %s: status %d, %v", path, status, err)
		}
		t.Cleanup(func() { _ = connection.Close() })
		return connection
	}
}

func readPhone(t *testing.T, connection *websocket.Conn) phoneFrame {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame phoneFrame
	if err := connection.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func waitHost(t *testing.T, requests <-chan hostFrame, kind string) hostFrame {
	t.Helper()
	select {
	case frame := <-requests:
		if frame.Params.Type != kind {
			t.Fatalf("host got %s, want %s", frame.Params.Type, kind)
		}
		return frame
	case <-time.After(2 * time.Second):
		t.Fatalf("host did not receive %s", kind)
		return hostFrame{}
	}
}

func TestClaudeRelayUsesAuthenticatedDesktopAndIsolatesOwnedCalls(t *testing.T) {
	f := newRelayFixture(t)
	host := f.dial(t, "/v1/voice/desktop", f.desktop.Token)
	var writeMu sync.Mutex
	writeHost := func(value any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = host.WriteJSON(value)
	}
	requests := make(chan hostFrame, 32)
	go func() {
		for {
			var frame hostFrame
			_, data, err := host.ReadMessage()
			if err != nil || json.Unmarshal(data, &frame) != nil {
				return
			}
			frame.Wire = string(data)
			requests <- frame
			value := any(nil)
			if frame.Params.Type == "start" {
				value = "v=0\r\nfixture-answer"
			} else if frame.Params.Type == "status" {
				value = live.ClaudeStatus{Available: true}
			}
			writeHost(map[string]any{"id": frame.ID, "result": value})
		}
	}()
	request, _ := http.NewRequest(http.MethodGet, f.server.URL+"/v1/voice/claude/status", nil)
	request.Header.Set("Authorization", "Bearer "+f.phones[0].Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var status live.ClaudeStatus
	_ = json.NewDecoder(response.Body).Decode(&status)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !status.Available {
		t.Fatalf("desktop status = %d %#v", response.StatusCode, status)
	}
	waitHost(t, requests, "status")
	path := "/v1/sessions/" + f.session + "/voice/claude"
	a := f.dial(t, path, f.phones[0].Token)
	b := f.dial(t, path, f.phones[1].Token)
	start := func(connection *websocket.Conn) hostFrame {
		t.Helper()
		_ = connection.WriteJSON(map[string]any{"id": "start", "command": map[string]any{"type": "start", "id": "caller-chosen-id", "options": map[string]string{"sdp": "v=0\r\noffer", "context": "Raw mobile chat context", "voice": "untrusted"}}})
		command := waitHost(t, requests, "start")
		if command.Method != "Claude.voice" || command.Params.ID == "caller-chosen-id" || command.Params.Options.Voice != "buttery" || !strings.HasPrefix(command.Params.Options.Context, "You are Jaz,") || !strings.HasSuffix(command.Params.Options.Context, "Raw mobile chat context") {
			t.Fatalf("noncanonical start reached desktop: %#v", command)
		}
		frame := readPhone(t, connection)
		if frame.ID != "start" || string(frame.Value) != `"v=0\r\nfixture-answer"` || frame.Error != "" {
			t.Fatalf("start response=%#v", frame)
		}
		return command
	}
	callA := start(a)
	callB := start(b)
	if callA.Params.ID == callB.Params.ID {
		t.Fatal("mobile sockets share a provider call identity")
	}
	event := func(id, data string) {
		writeHost(map[string]any{"method": "Claude.voiceEvent", "params": map[string]string{"id": id, "data": data}})
	}
	event(callA.Params.ID, `{"type":"message_complete","text":"only A"}`)
	event(callB.Params.ID, `{"type":"message_complete","text":"only B"}`)
	if frame := readPhone(t, a); frame.Type != "event" || !strings.Contains(frame.Data, "only A") {
		t.Fatalf("call A event=%#v", frame)
	}
	if frame := readPhone(t, b); frame.Type != "event" || !strings.Contains(frame.Data, "only B") {
		t.Fatalf("call B received another call's event: %#v", frame)
	}
	_ = b.WriteJSON(map[string]any{"id": "result", "command": map[string]any{"type": "result", "id": callA.Params.ID, "toolUseId": "tool-b", "text": "Accepted", "isError": false}})
	command := waitHost(t, requests, "result")
	if command.Params.ID != callB.Params.ID || command.Params.ToolUseID != "tool-b" || !strings.Contains(command.Wire, `"isError":false`) {
		t.Fatalf("caller could select another provider call: %#v", command)
	}
	if frame := readPhone(t, b); frame.ID != "result" || frame.Error != "" {
		t.Fatalf("tool result response=%#v", frame)
	}
	if _, err := f.devices.RevokeDevice(f.phones[0].Device.ID); err != nil {
		t.Fatal(err)
	}
	if frame := waitHost(t, requests, "close"); frame.Params.ID != callA.Params.ID {
		t.Fatalf("revocation closed another call: %#v", frame)
	}
	event(callB.Params.ID, `{"type":"message_complete","text":"B survives A revocation"}`)
	if frame := readPhone(t, b); !strings.Contains(frame.Data, "B survives") {
		t.Fatalf("other mobile call was interrupted: %#v", frame)
	}
	connection, rejected, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+path, http.Header{"Authorization": {"Bearer " + f.phones[0].Token}})
	if connection != nil {
		connection.Close()
	}
	if err == nil || rejected == nil || rejected.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked phone reconnected: %v %#v", err, rejected)
	}
	if _, err := f.devices.RevokeDevice(f.desktop.Device.ID); err != nil {
		t.Fatal(err)
	}
	if frame := readPhone(t, b); frame.Type != "event" || !strings.Contains(frame.Data, "jaz_disconnected") {
		t.Fatalf("host disconnect was not surfaced: %#v", frame)
	}
}

func TestClaudeRelayRejectsUnauthenticatedAndMobileHosts(t *testing.T) {
	f := newRelayFixture(t)
	for _, test := range []struct {
		path, token string
		status      int
	}{
		{"/v1/voice/desktop", "", http.StatusUnauthorized},
		{"/v1/voice/desktop", f.phones[0].Token, http.StatusForbidden},
		{"/v1/sessions/" + f.session + "/voice/claude", "", http.StatusUnauthorized},
		{"/v1/sessions/missing/voice/claude", f.phones[0].Token, http.StatusNotFound},
		{"/v1/sessions/" + f.session + "/voice/claude", f.phones[0].Token, http.StatusServiceUnavailable},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer "+test.token)
		result := httptest.NewRecorder()
		f.server.Config.Handler.ServeHTTP(result, request)
		if result.Code != test.status {
			t.Fatalf("%s status=%d, want %d", test.path, result.Code, test.status)
		}
	}
}

func TestClaudeRelayDisconnectDuringSetupReleasesOnlyItsCall(t *testing.T) {
	for _, closed := range []string{"mobile", "desktop", "invalid answer"} {
		t.Run(closed, func(t *testing.T) {
			f := newRelayFixture(t)
			host := f.dial(t, "/v1/voice/desktop", f.desktop.Token)
			requests := make(chan hostFrame, 4)
			go func() {
				for {
					var frame hostFrame
					if host.ReadJSON(&frame) != nil {
						return
					}
					requests <- frame
					if frame.Params.Type == "close" {
						_ = host.WriteJSON(map[string]any{"id": frame.ID, "result": nil})
					} else if closed == "invalid answer" {
						_ = host.WriteJSON(map[string]any{"id": frame.ID, "result": "not SDP"})
					}
				}
			}()
			phone := f.dial(t, "/v1/sessions/"+f.session+"/voice/claude", f.phones[0].Token)
			_ = phone.WriteJSON(map[string]any{"id": "start", "command": map[string]any{"type": "start", "options": map[string]string{"sdp": "v=0\r\noffer", "context": "Mobile context"}}})
			start := waitHost(t, requests, "start")
			if closed == "mobile" {
				phone.Close()
			} else if closed == "desktop" {
				host.Close()
				frame := readPhone(t, phone)
				if !strings.Contains(frame.Data, "jaz_disconnected") && !strings.Contains(frame.Error, "Jaz Desktop") {
					t.Fatalf("lost host did not fail setup: %#v", frame)
				}
				return
			} else {
				frame := readPhone(t, phone)
				if !strings.Contains(frame.Error, "invalid voice connection answer") || len(frame.Value) != 0 {
					t.Fatalf("invalid SDP was forwarded: %#v", frame)
				}
			}
			if frame := waitHost(t, requests, "close"); frame.Params.ID != start.Params.ID {
				t.Fatalf("setup cleanup closed another call: %#v", frame)
			}
		})
	}
}
