package deviceauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var (
	ErrUnauthorized     = errors.New("device token is missing or invalid")
	ErrApprovalRequired = errors.New("device approval required")
	ErrForbidden        = errors.New("device is not allowed")
	ErrPairingExpired   = errors.New("pairing request expired")
)

const (
	PrincipalRoot   = "root"
	PrincipalDevice = "device"
)

const pairingTTL = 10 * time.Minute

type Store interface {
	ListDevices() ([]storage.Device, error)
	CountApprovedDevices() (int, error)
	LoadDeviceByTokenHash(string) (storage.Device, error)
	CreateDevice(storage.CreateDevice) (storage.Device, error)
	UpdateDeviceSeen(id, ip, userAgent string, at time.Time) error
	RenameDevice(id, name string) (storage.Device, error)
	RevokeDevice(id string, at time.Time) (storage.Device, error)
	CreateDevicePairing(storage.CreateDevicePairing) (storage.DevicePairing, error)
	LoadDevicePairing(id string) (storage.DevicePairing, string, error)
	ListDevicePairings() ([]storage.DevicePairing, error)
	ApproveDevicePairing(id string, at time.Time) (storage.DevicePairing, error)
	RejectDevicePairing(id string, at time.Time) (storage.DevicePairing, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

type ClientInfo struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	AppVersion string `json:"app_version,omitempty"`
	IP         string `json:"-"`
	UserAgent  string `json:"-"`
}

type Principal struct {
	Kind     string `json:"kind"`
	DeviceID string `json:"device_id,omitempty"`
}

type RegisterResult struct {
	Device        storage.Device         `json:"device"`
	Token         string                 `json:"token,omitempty"`
	Pairing       *storage.DevicePairing `json:"pairing,omitempty"`
	PairingSecret string                 `json:"pairing_secret,omitempty"`
}

type PairingPoll struct {
	Pairing storage.DevicePairing `json:"pairing"`
	Token   string                `json:"token,omitempty"`
}

type contextKey struct{}

func New(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func (s *Service) ApprovedDeviceCount() (int, error) {
	return s.store.CountApprovedDevices()
}

func (s *Service) Authenticate(token string, info ClientInfo) (Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Principal{}, ErrUnauthorized
	}
	device, err := s.store.LoadDeviceByTokenHash(TokenHash(token))
	if err != nil {
		return Principal{}, ErrUnauthorized
	}
	switch device.Status {
	case storage.DeviceStatusApproved:
		_ = s.store.UpdateDeviceSeen(device.ID, info.IP, info.UserAgent, s.now())
		return Principal{Kind: PrincipalDevice, DeviceID: device.ID}, nil
	case storage.DeviceStatusPending:
		return Principal{}, ErrApprovalRequired
	default:
		return Principal{}, ErrForbidden
	}
}

func (s *Service) Register(info ClientInfo) (RegisterResult, error) {
	count, err := s.store.CountApprovedDevices()
	if err != nil {
		return RegisterResult{}, err
	}
	if count == 0 {
		return s.createApprovedDevice(info)
	}
	pairing, secret, err := s.CreatePairing(info)
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Device: pairing.Device, Pairing: &pairing, PairingSecret: secret}, nil
}

func (s *Service) CreatePairing(info ClientInfo) (storage.DevicePairing, string, error) {
	info = normalizeClientInfo(info)
	secret, err := newToken()
	if err != nil {
		return storage.DevicePairing{}, "", err
	}
	now := s.now()
	deviceID := newID("dev")
	device, err := s.store.CreateDevice(storage.CreateDevice{
		ID:         deviceID,
		Name:       info.Name,
		Kind:       info.Kind,
		Status:     storage.DeviceStatusPending,
		TokenHash:  TokenHash(secret),
		CreatedAt:  now,
		LastSeenIP: info.IP,
		UserAgent:  info.UserAgent,
		AppVersion: info.AppVersion,
	})
	if err != nil {
		return storage.DevicePairing{}, "", err
	}
	pairing, err := s.store.CreateDevicePairing(storage.CreateDevicePairing{
		ID:         newID("pair"),
		DeviceID:   device.ID,
		SecretHash: TokenHash(secret),
		Status:     storage.PairingStatusPending,
		CreatedAt:  now,
		ExpiresAt:  now.Add(pairingTTL),
	})
	return pairing, secret, err
}

func (s *Service) PollPairing(id, secret string) (PairingPoll, error) {
	pairing, secretHash, err := s.store.LoadDevicePairing(strings.TrimSpace(id))
	if err != nil {
		return PairingPoll{}, err
	}
	if subtle.ConstantTimeCompare([]byte(secretHash), []byte(TokenHash(secret))) != 1 {
		return PairingPoll{}, ErrUnauthorized
	}
	if pairing.Status == storage.PairingStatusPending && s.now().After(pairing.ExpiresAt) {
		return PairingPoll{Pairing: pairing}, ErrPairingExpired
	}
	if pairing.Status == storage.PairingStatusApproved {
		return PairingPoll{Pairing: pairing, Token: strings.TrimSpace(secret)}, nil
	}
	return PairingPoll{Pairing: pairing}, nil
}

func (s *Service) ApprovePairing(id string) (storage.DevicePairing, error) {
	return s.store.ApproveDevicePairing(strings.TrimSpace(id), s.now())
}

func (s *Service) RejectPairing(id string) (storage.DevicePairing, error) {
	return s.store.RejectDevicePairing(strings.TrimSpace(id), s.now())
}

func (s *Service) RenameDevice(id, name string) (storage.Device, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return storage.Device{}, fmt.Errorf("device name is required")
	}
	return s.store.RenameDevice(strings.TrimSpace(id), name)
}

func (s *Service) RevokeDevice(id string) (storage.Device, error) {
	return s.store.RevokeDevice(strings.TrimSpace(id), s.now())
}

func (s *Service) List() ([]storage.Device, []storage.DevicePairing, error) {
	devices, err := s.store.ListDevices()
	if err != nil {
		return nil, nil, err
	}
	pairings, err := s.store.ListDevicePairings()
	if err != nil {
		return nil, nil, err
	}
	return devices, pairings, nil
}

func ClientInfoFromRequest(r *http.Request, fallbackName string) ClientInfo {
	name := strings.TrimSpace(r.Header.Get("X-Jaz-Device-Name"))
	if name == "" {
		name = fallbackName
	}
	return ClientInfo{
		Name:       name,
		Kind:       strings.TrimSpace(r.Header.Get("X-Jaz-Device-Kind")),
		AppVersion: strings.TrimSpace(r.Header.Get("X-Jaz-App-Version")),
		IP:         requestIP(r),
		UserAgent:  strings.TrimSpace(r.UserAgent()),
	}
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func normalizeClientInfo(info ClientInfo) ClientInfo {
	info.Name = strings.TrimSpace(info.Name)
	if info.Name == "" {
		info.Name = "Jaz device"
	}
	info.Kind = strings.TrimSpace(info.Kind)
	switch info.Kind {
	case storage.DeviceKindDesktop, storage.DeviceKindMobile, storage.DeviceKindBrowser, storage.DeviceKindCLI:
	default:
		info.Kind = storage.DeviceKindDesktop
	}
	info.AppVersion = strings.TrimSpace(info.AppVersion)
	info.IP = strings.TrimSpace(info.IP)
	info.UserAgent = strings.TrimSpace(info.UserAgent)
	return info
}

func (s *Service) createApprovedDevice(info ClientInfo) (RegisterResult, error) {
	info = normalizeClientInfo(info)
	token, err := newToken()
	if err != nil {
		return RegisterResult{}, err
	}
	now := s.now()
	device, err := s.store.CreateDevice(storage.CreateDevice{
		ID:         newID("dev"),
		Name:       info.Name,
		Kind:       info.Kind,
		Status:     storage.DeviceStatusApproved,
		TokenHash:  TokenHash(token),
		CreatedAt:  now,
		ApprovedAt: now,
		LastSeenAt: now,
		LastSeenIP: info.IP,
		UserAgent:  info.UserAgent,
		AppVersion: info.AppVersion,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Device: device, Token: token}, nil
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "jaz_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b[:])
}

func requestIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		raw := strings.TrimSpace(r.Header.Get(header))
		if raw == "" {
			continue
		}
		host := strings.TrimSpace(strings.Split(raw, ",")[0])
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(host)
}
