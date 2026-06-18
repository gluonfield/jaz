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
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/storage"
)

type Handler struct {
	service      *deviceauth.Service
	serverConfig serverconfig.Config
}

type deviceInput struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	DeviceID   string `json:"device_id"`
	PublicKey  string `json:"public_key"`
	Platform   string `json:"platform"`
	Family     string `json:"device_family"`
	Model      string `json:"model_identifier"`
	AppVersion string `json:"app_version"`
}

type listResponse struct {
	Devices         []deviceDTO  `json:"devices"`
	Pairings        []pairingDTO `json:"pairings"`
	CurrentDeviceID string       `json:"current_device_id,omitempty"`
}

type connectionLinkResponse struct {
	URL string `json:"url"`
}

type deviceDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Platform   string     `json:"platform,omitempty"`
	Family     string     `json:"device_family,omitempty"`
	Model      string     `json:"model_identifier,omitempty"`
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

func NewHandler(service *deviceauth.Service, config serverconfig.Config) Handler {
	return Handler{service: service, serverConfig: config}
}

func (h Handler) ConnectionLink(w http.ResponseWriter, r *http.Request) {
	cfg := h.serverConfig
	if strings.TrimSpace(cfg.PublicURL) == "" {
		cfg.PublicURL = httpapi.RequestBaseURL(r)
	}
	httpapi.WriteJSON(w, http.StatusOK, connectionLinkResponse{
		URL: serverconfig.ClientBaseURL(cfg),
	})
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
	result, err := h.register(r, info)
	if err != nil {
		writeDeviceAuthError(w, err)
		return
	}
	status := http.StatusOK
	if result.Token == "" {
		status = http.StatusAccepted
	}
	httpapi.WriteJSON(w, status, registerDTO(result))
}

func (h Handler) register(r *http.Request, info deviceauth.Registration) (deviceauth.RegisterResult, error) {
	principal, _ := deviceauth.PrincipalFromContext(r.Context())
	if principal.Kind == deviceauth.PrincipalRoot {
		return h.service.RegisterApproved(info)
	}
	return h.service.Register(info)
}

func (h Handler) CreatePairing(w http.ResponseWriter, r *http.Request) {
	info, err := inputFromRequest(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	pairing, secret, err := h.service.CreatePairing(info)
	if err != nil {
		writeDeviceAuthError(w, err)
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

func (h Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httpapi.WriteError(w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}
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

func inputFromRequest(r *http.Request) (deviceauth.Registration, error) {
	info := httpapi.RequestInfoFrom(r)
	reg := deviceauth.Registration{
		Identity: deviceauth.DeviceIdentity{
			DeviceID:  strings.TrimSpace(r.Header.Get("X-Jaz-Device-ID")),
			PublicKey: strings.TrimSpace(r.Header.Get("X-Jaz-Device-Public-Key")),
		},
		Profile: deviceauth.DeviceProfile{
			Name:       deviceNameFromHeader(r, "Jaz desktop"),
			Kind:       strings.TrimSpace(r.Header.Get("X-Jaz-Device-Kind")),
			Platform:   strings.TrimSpace(r.Header.Get("X-Jaz-Device-Platform")),
			Family:     strings.TrimSpace(r.Header.Get("X-Jaz-Device-Family")),
			Model:      strings.TrimSpace(r.Header.Get("X-Jaz-Device-Model")),
			AppVersion: strings.TrimSpace(r.Header.Get("X-Jaz-App-Version")),
		},
		Seen: deviceauth.SeenInfo{IP: info.IP, UserAgent: info.UserAgent},
	}
	if r.Body == nil {
		return reg, nil
	}
	var input deviceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		if errors.Is(err, io.EOF) {
			return reg, nil
		}
		return deviceauth.Registration{}, err
	}
	if strings.TrimSpace(input.Name) != "" {
		reg.Profile.Name = input.Name
	}
	if strings.TrimSpace(input.Kind) != "" {
		reg.Profile.Kind = input.Kind
	}
	if strings.TrimSpace(input.DeviceID) != "" {
		reg.Identity.DeviceID = input.DeviceID
	}
	if strings.TrimSpace(input.PublicKey) != "" {
		reg.Identity.PublicKey = input.PublicKey
	}
	if strings.TrimSpace(input.Platform) != "" {
		reg.Profile.Platform = input.Platform
	}
	if strings.TrimSpace(input.Family) != "" {
		reg.Profile.Family = input.Family
	}
	if strings.TrimSpace(input.Model) != "" {
		reg.Profile.Model = input.Model
	}
	if strings.TrimSpace(input.AppVersion) != "" {
		reg.Profile.AppVersion = input.AppVersion
	}
	return reg, nil
}

func deviceNameFromHeader(r *http.Request, fallback string) string {
	name := strings.TrimSpace(r.Header.Get("X-Jaz-Device-Name"))
	if name == "" {
		return fallback
	}
	return name
}

func writeDeviceAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, deviceauth.ErrInvalidIdentity) {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	httpapi.WriteError(w, http.StatusInternalServerError, err)
}

func pairingPath(path string) (string, string, error) {
	rest := strings.Trim(strings.TrimPrefix(path, "/v1/devices/pairing-requests/"), "/")
	if rest == "" || rest == path {
		return "", "", fmt.Errorf("not found")
	}
	id, action, _ := strings.Cut(rest, "/")
	return strings.TrimSpace(id), strings.TrimSpace(action), nil
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
		Platform:   device.Platform,
		Family:     device.Family,
		Model:      device.Model,
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
