package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/desktopbridge"
	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/voice/live"
)

type relayStore interface {
	LoadSession(string) (storage.Session, error)
	LoadDevice(string) (storage.Device, error)
}

type RelayHandler struct {
	service *live.Service
	relay   *live.DesktopRelay
	store   relayStore
}

type relayRequest struct {
	ID      string             `json:"id"`
	Command live.ClaudeCommand `json:"command"`
}

type relayResponse struct {
	ID    string          `json:"id,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
	Type  string          `json:"type,omitempty"`
	Data  string          `json:"data,omitempty"`
}

func NewRelayHandler(store relayStore, service *live.Service, relay *live.DesktopRelay) *RelayHandler {
	return &RelayHandler{service: service, relay: relay, store: store}
}

func (h *RelayHandler) Desktop(w http.ResponseWriter, r *http.Request) {
	principal, ok := deviceauth.PrincipalFromContext(r.Context())
	if !ok || principal.Kind != deviceauth.PrincipalDevice {
		httpapi.WriteError(w, http.StatusForbidden, errors.New("an approved Jaz Desktop device is required"))
		return
	}
	device, err := h.store.LoadDevice(principal.DeviceID)
	if err != nil || device.Kind != storage.DeviceKindDesktop || device.Status != storage.DeviceStatusApproved {
		httpapi.WriteError(w, http.StatusForbidden, errors.New("an approved Jaz Desktop device is required"))
		return
	}
	conn, err := httpapi.UpgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
	defer stop()
	h.relay.Connect(r.Context(), conn)
}

func (h *RelayHandler) Status(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, h.relay.Status(r.Context()))
}

func (h *RelayHandler) Call(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session")
	if _, err := h.store.LoadSession(sessionID); err != nil {
		writeError(w, err)
		return
	}
	call, err := h.relay.Open()
	if err != nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, err)
		return
	}
	defer call.Close()
	conn, err := httpapi.UpgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	desktopbridge.KeepAlive(conn, ctx.Done())
	setup := time.AfterFunc(45*time.Second, cancel)
	defer setup.Stop()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	conn.SetReadLimit(256 * 1024)
	var writeMu sync.Mutex
	write := func(response relayResponse) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(response)
	}
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-call.Done():
				_ = write(relayResponse{Type: "event", Data: `{"type":"jaz_disconnected"}`})
				return
			case data := <-call.Events():
				if write(relayResponse{Type: "event", Data: data}) != nil {
					return
				}
			}
		}
	}()
	requests := make(chan relayRequest, 8)
	go func() {
		defer cancel()
		for {
			var request relayRequest
			if conn.ReadJSON(&request) != nil {
				return
			}
			select {
			case requests <- request:
			case <-ctx.Done():
				return
			default:
				return
			}
		}
	}()
	started := false
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-requests:
			result, err := h.command(ctx, sessionID, call, request, &started)
			if request.Command.Type == "start" && err == nil {
				setup.Stop()
			}
			response := relayResponse{ID: request.ID, Value: result}
			if err != nil {
				response.Error = err.Error()
			}
			if write(response) != nil || request.Command.Type == "close" || request.Command.Type == "start" && err != nil {
				return
			}
		}
	}
}

func (h *RelayHandler) command(ctx context.Context, session string, call *live.DesktopCall, request relayRequest, started *bool) (json.RawMessage, error) {
	if request.ID == "" || len(request.ID) > 128 {
		return nil, errors.New("invalid voice request identity")
	}
	command := request.Command
	switch command.Type {
	case "start":
		if *started || command.Options == nil {
			return nil, errors.New("voice has already started or its offer is missing")
		}
		*started = true
		options, err := h.service.PrepareClaude(command.Options.SDP, command.Options.Context, session)
		if err != nil {
			return nil, err
		}
		command.Options = &options
	case "send":
		if !*started || len(command.Data) > 128*1024 || !json.Valid([]byte(command.Data)) {
			return nil, errors.New("invalid Claude voice message")
		}
	case "result":
		if !*started || command.ToolUseID == "" || len(command.ToolUseID) > 128 || len(command.Text) > 64*1024 {
			return nil, errors.New("invalid Claude voice tool result")
		}
	case "close":
	default:
		return nil, errors.New("unsupported Claude voice command")
	}
	result, err := call.Command(ctx, command)
	if err == nil && command.Type == "start" {
		var sdp string
		if json.Unmarshal(result, &sdp) != nil || !strings.HasPrefix(sdp, "v=0\r\n") && !strings.HasPrefix(sdp, "v=0\n") || len(sdp) > 128*1024 {
			return nil, errors.New("Claude returned an invalid voice connection answer")
		}
	}
	return result, err
}
