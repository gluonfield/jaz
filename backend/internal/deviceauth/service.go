package deviceauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var (
	ErrUnauthorized     = errors.New("device token is missing or invalid")
	ErrApprovalRequired = errors.New("device approval required")
	ErrForbidden        = errors.New("device is not allowed")
	ErrPairingExpired   = errors.New("pairing request expired")
	ErrInvalidIdentity  = errors.New("device identity is invalid")
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
	SavePairingDevice(storage.SavePairingDevice) (storage.Device, error)
	UpdateDeviceSeen(id, ip, userAgent string, at time.Time) error
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

type SeenInfo struct {
	IP        string
	UserAgent string
}

type DeviceIdentity struct {
	DeviceID  string
	PublicKey string
}

type DeviceProfile struct {
	Name       string
	Kind       string
	Platform   string
	Family     string
	Model      string
	AppVersion string
}

type Registration struct {
	Identity DeviceIdentity
	Profile  DeviceProfile
	Seen     SeenInfo
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

func (s *Service) Authenticate(token string, seen SeenInfo) (Principal, error) {
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
		seen = normalizeSeenInfo(seen)
		_ = s.store.UpdateDeviceSeen(device.ID, seen.IP, seen.UserAgent, s.now())
		return Principal{Kind: PrincipalDevice, DeviceID: device.ID}, nil
	case storage.DeviceStatusPending:
		return Principal{}, ErrApprovalRequired
	default:
		return Principal{}, ErrForbidden
	}
}

func (s *Service) Register(reg Registration) (RegisterResult, error) {
	var err error
	reg, err = normalizeRegistration(reg)
	if err != nil {
		return RegisterResult{}, err
	}
	count, err := s.store.CountApprovedDevices()
	if err != nil {
		return RegisterResult{}, err
	}
	if count == 0 {
		return s.createApprovedDevice(reg)
	}
	pairing, secret, err := s.createPairing(reg)
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Device: pairing.Device, Pairing: &pairing, PairingSecret: secret}, nil
}

func (s *Service) CreatePairing(reg Registration) (storage.DevicePairing, string, error) {
	var err error
	reg, err = normalizeRegistration(reg)
	if err != nil {
		return storage.DevicePairing{}, "", err
	}
	return s.createPairing(reg)
}

func (s *Service) createPairing(reg Registration) (storage.DevicePairing, string, error) {
	secret, err := newToken()
	if err != nil {
		return storage.DevicePairing{}, "", err
	}
	now := s.now()
	device, err := s.store.SavePairingDevice(storage.SavePairingDevice{
		ID:         reg.Identity.DeviceID,
		Name:       reg.Profile.Name,
		Kind:       reg.Profile.Kind,
		PublicKey:  reg.Identity.PublicKey,
		Platform:   reg.Profile.Platform,
		Family:     reg.Profile.Family,
		Model:      reg.Profile.Model,
		TokenHash:  TokenHash(secret),
		CreatedAt:  now,
		LastSeenIP: reg.Seen.IP,
		UserAgent:  reg.Seen.UserAgent,
		AppVersion: reg.Profile.AppVersion,
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

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func normalizeSeenInfo(seen SeenInfo) SeenInfo {
	seen.IP = strings.TrimSpace(seen.IP)
	seen.UserAgent = strings.TrimSpace(seen.UserAgent)
	return seen
}

func normalizeProfile(profile DeviceProfile) DeviceProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" {
		profile.Name = "Jaz device"
	}
	profile.Kind = strings.TrimSpace(profile.Kind)
	switch profile.Kind {
	case storage.DeviceKindDesktop, storage.DeviceKindMobile, storage.DeviceKindBrowser, storage.DeviceKindCLI:
	default:
		profile.Kind = storage.DeviceKindDesktop
	}
	profile.AppVersion = strings.TrimSpace(profile.AppVersion)
	profile.Platform = strings.TrimSpace(profile.Platform)
	profile.Family = strings.TrimSpace(profile.Family)
	profile.Model = strings.TrimSpace(profile.Model)
	return profile
}

func normalizeRegistration(reg Registration) (Registration, error) {
	reg.Profile = normalizeProfile(reg.Profile)
	reg.Seen = normalizeSeenInfo(reg.Seen)
	publicKey, deviceID, err := normalizeDeviceIdentity(reg.Identity.PublicKey, reg.Identity.DeviceID)
	if err != nil {
		return Registration{}, err
	}
	reg.Identity.PublicKey = publicKey
	reg.Identity.DeviceID = deviceID
	return reg, nil
}

func (s *Service) createApprovedDevice(reg Registration) (RegisterResult, error) {
	token, err := newToken()
	if err != nil {
		return RegisterResult{}, err
	}
	now := s.now()
	device, err := s.store.CreateDevice(storage.CreateDevice{
		ID:         reg.Identity.DeviceID,
		Name:       reg.Profile.Name,
		Kind:       reg.Profile.Kind,
		Status:     storage.DeviceStatusApproved,
		PublicKey:  reg.Identity.PublicKey,
		Platform:   reg.Profile.Platform,
		Family:     reg.Profile.Family,
		Model:      reg.Profile.Model,
		TokenHash:  TokenHash(token),
		CreatedAt:  now,
		ApprovedAt: now,
		LastSeenAt: now,
		LastSeenIP: reg.Seen.IP,
		UserAgent:  reg.Seen.UserAgent,
		AppVersion: reg.Profile.AppVersion,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Device: device, Token: token}, nil
}

func normalizeDeviceIdentity(publicKey, deviceID string) (string, string, error) {
	raw, err := decodeRawPublicKey(publicKey)
	if err != nil || len(raw) != 32 {
		return "", "", ErrInvalidIdentity
	}
	sum := sha256.Sum256(raw)
	expected := hex.EncodeToString(sum[:])
	deviceID = strings.ToLower(strings.TrimSpace(deviceID))
	if deviceID != expected {
		return "", "", ErrInvalidIdentity
	}
	return base64.RawURLEncoding.EncodeToString(raw), expected, nil
}

func decodeRawPublicKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ErrInvalidIdentity
	}
	if raw, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return raw, nil
	}
	return base64.URLEncoding.DecodeString(value)
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
