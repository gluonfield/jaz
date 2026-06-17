package devices

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

type Handler struct {
	service *deviceauth.Service
}

type deviceInput struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	AppVersion string `json:"app_version"`
}

type renameInput struct {
	Name string `json:"name"`
}

type listResponse struct {
	Devices         []deviceDTO  `json:"devices"`
	Pairings        []pairingDTO `json:"pairings"`
	CurrentDeviceID string       `json:"current_device_id,omitempty"`
}

type deviceDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	LastSeenIP string     `json:"last_seen_ip,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
	AppVersion string     `json:"app_version,omitempty"`
}

type pairingDTO struct {
	ID         string     `json:"id"`
	DeviceID   string     `json:"device_id"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	Device     deviceDTO  `json:"device"`
}

func NewHandler(service *deviceauth.Service) Handler {
	return Handler{service: service}
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	devices, pairings, err := h.service.List()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	principal, _ := deviceauth.PrincipalFromContext(r.Context())
	httpapi.WriteJSON(w, http.StatusOK, listResponse{
		Devices:         deviceDTOs(devices),
		Pairings:        pairingDTOs(pairings),
		CurrentDeviceID: principal.DeviceID,
	})
}

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	info, err := inputFromRequest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.Register(info)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	status := http.StatusOK
	if result.Token == "" {
		status = http.StatusAccepted
	}
	httpapi.WriteJSON(w, status, registerDTO(result))
}

func (h Handler) CreatePairing(w http.ResponseWriter, r *http.Request) {
	info, err := inputFromRequest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	pairing, secret, err := h.service.CreatePairing(info)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]any{
		"pairing":        pairingDTOFromStorage(pairing),
		"pairing_secret": secret,
	})
}

func (h Handler) Pairing(w http.ResponseWriter, r *http.Request) {
	id, action, err := pairingPath(r.URL.Path)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, err)
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		h.pollPairing(w, r, id)
	case r.Method == http.MethodPost && action == "approve":
		pairing, err := h.service.ApprovePairing(id)
		writePairingAction(w, pairing, err)
	case r.Method == http.MethodPost && action == "reject":
		pairing, err := h.service.RejectPairing(id)
		writePairingAction(w, pairing, err)
	default:
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func (h Handler) Device(w http.ResponseWriter, r *http.Request) {
	id, err := devicePath(r.URL.Path)
	if err != nil {
		httpapi.WriteError(w, http.StatusNotFound, err)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var input renameInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		device, err := h.service.RenameDevice(id, input.Name)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"device": deviceDTOFromStorage(device)})
	case http.MethodDelete:
		if principal, ok := deviceauth.PrincipalFromContext(r.Context()); ok && principal.DeviceID == id {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("cannot revoke the current device"))
			return
		}
		device, err := h.service.RevokeDevice(id)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"device": deviceDTOFromStorage(device)})
	default:
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("not found"))
	}
}

func (h Handler) pollPairing(w http.ResponseWriter, r *http.Request, id string) {
	secret := strings.TrimSpace(r.URL.Query().Get("secret"))
	if secret == "" {
		secret = strings.TrimSpace(r.Header.Get("X-Jaz-Pairing-Secret"))
	}
	poll, err := h.service.PollPairing(id, secret)
	switch {
	case errors.Is(err, deviceauth.ErrUnauthorized):
		httpapi.WriteError(w, http.StatusUnauthorized, err)
	case errors.Is(err, deviceauth.ErrPairingExpired):
		httpapi.WriteJSON(w, http.StatusGone, pairingPollDTO(poll))
	case err != nil:
		httpapi.WriteError(w, http.StatusBadRequest, err)
	default:
		httpapi.WriteJSON(w, http.StatusOK, pairingPollDTO(poll))
	}
}

func writePairingAction(w http.ResponseWriter, pairing storage.DevicePairing, err error) {
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"pairing": pairingDTOFromStorage(pairing)})
}

func inputFromRequest(r *http.Request) (deviceauth.ClientInfo, error) {
	info := deviceauth.ClientInfoFromRequest(r, "Jaz desktop")
	if r.Body == nil {
		return info, nil
	}
	var input deviceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		if errors.Is(err, io.EOF) {
			return info, nil
		}
		return deviceauth.ClientInfo{}, err
	}
	if strings.TrimSpace(input.Name) != "" {
		info.Name = input.Name
	}
	if strings.TrimSpace(input.Kind) != "" {
		info.Kind = input.Kind
	}
	if strings.TrimSpace(input.AppVersion) != "" {
		info.AppVersion = input.AppVersion
	}
	return info, nil
}

func pairingPath(path string) (string, string, error) {
	rest := strings.Trim(strings.TrimPrefix(path, "/v1/devices/pairing-requests/"), "/")
	if rest == "" || rest == path {
		return "", "", fmt.Errorf("not found")
	}
	id, action, _ := strings.Cut(rest, "/")
	return strings.TrimSpace(id), strings.TrimSpace(action), nil
}

func devicePath(path string) (string, error) {
	rest := strings.Trim(strings.TrimPrefix(path, "/v1/devices/"), "/")
	if rest == "" || rest == path || strings.HasPrefix(rest, "pairing-requests") {
		return "", fmt.Errorf("not found")
	}
	id, _, _ := strings.Cut(rest, "/")
	return strings.TrimSpace(id), nil
}

func registerDTO(result deviceauth.RegisterResult) map[string]any {
	out := map[string]any{"device": deviceDTOFromStorage(result.Device)}
	if result.Token != "" {
		out["token"] = result.Token
	}
	if result.Pairing != nil {
		out["pairing"] = pairingDTOFromStorage(*result.Pairing)
	}
	if result.PairingSecret != "" {
		out["pairing_secret"] = result.PairingSecret
	}
	return out
}

func pairingPollDTO(poll deviceauth.PairingPoll) map[string]any {
	out := map[string]any{"pairing": pairingDTOFromStorage(poll.Pairing)}
	if poll.Token != "" {
		out["token"] = poll.Token
	}
	return out
}

func deviceDTOs(devices []storage.Device) []deviceDTO {
	out := make([]deviceDTO, 0, len(devices))
	for _, device := range devices {
		out = append(out, deviceDTOFromStorage(device))
	}
	return out
}

func pairingDTOs(pairings []storage.DevicePairing) []pairingDTO {
	out := make([]pairingDTO, 0, len(pairings))
	for _, pairing := range pairings {
		out = append(out, pairingDTOFromStorage(pairing))
	}
	return out
}

func deviceDTOFromStorage(device storage.Device) deviceDTO {
	return deviceDTO{
		ID:         device.ID,
		Name:       device.Name,
		Kind:       device.Kind,
		Status:     device.Status,
		CreatedAt:  device.CreatedAt,
		ApprovedAt: timePtr(device.ApprovedAt),
		RevokedAt:  timePtr(device.RevokedAt),
		LastSeenAt: timePtr(device.LastSeenAt),
		LastSeenIP: device.LastSeenIP,
		UserAgent:  device.UserAgent,
		AppVersion: device.AppVersion,
	}
}

func pairingDTOFromStorage(pairing storage.DevicePairing) pairingDTO {
	return pairingDTO{
		ID:         pairing.ID,
		DeviceID:   pairing.DeviceID,
		Status:     pairing.Status,
		CreatedAt:  pairing.CreatedAt,
		ExpiresAt:  pairing.ExpiresAt,
		ApprovedAt: timePtr(pairing.ApprovedAt),
		RejectedAt: timePtr(pairing.RejectedAt),
		Device:     deviceDTOFromStorage(pairing.Device),
	}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
