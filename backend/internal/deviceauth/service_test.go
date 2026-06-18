package deviceauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestRegisterRequiresValidDeviceIdentity(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = New(store).Register(Registration{Profile: DeviceProfile{Name: "Mac", Kind: "desktop"}})
	if !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("Register error = %v, want ErrInvalidIdentity", err)
	}
}

func TestRegisterUsesClientDeviceIdentity(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	info := testClientInfo("First Mac", "desktop", 1)
	result, err := New(store).Register(info)
	if err != nil {
		t.Fatal(err)
	}
	if result.Device.ID != info.Identity.DeviceID {
		t.Fatalf("device id = %q, want %q", result.Device.ID, info.Identity.DeviceID)
	}
	if result.Device.PublicKey != info.Identity.PublicKey {
		t.Fatalf("public key = %q, want %q", result.Device.PublicKey, info.Identity.PublicKey)
	}
	if result.Device.Platform != info.Profile.Platform || result.Device.Family != info.Profile.Family || result.Device.Model != info.Profile.Model {
		t.Fatalf("device metadata = %q/%q/%q, want %q/%q/%q",
			result.Device.Platform,
			result.Device.Family,
			result.Device.Model,
			info.Profile.Platform,
			info.Profile.Family,
			info.Profile.Model,
		)
	}
}

func TestApprovedDeviceRepairRotatesTokenOnApproval(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store)
	info := testClientInfo("Mac", "desktop", 1)
	registered, err := service.Register(info)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(registered.Token, SeenInfo{}); err != nil {
		t.Fatalf("initial token auth: %v", err)
	}

	pairing, secret, err := service.CreatePairing(info)
	if err != nil {
		t.Fatal(err)
	}
	if pairing.Device.Status != "approved" {
		t.Fatalf("repair pairing device status = %q, want approved", pairing.Device.Status)
	}
	if _, err := service.Authenticate(registered.Token, SeenInfo{}); err != nil {
		t.Fatalf("pending repair changed old token: %v", err)
	}
	if _, err := service.ApprovePairing(pairing.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(registered.Token, SeenInfo{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old token auth error = %v, want ErrUnauthorized", err)
	}
	if _, err := service.Authenticate(secret, SeenInfo{}); err != nil {
		t.Fatalf("rotated token auth: %v", err)
	}
}

func TestRegisterApprovedApprovesDeviceAfterBootstrap(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store)
	if _, err := service.Register(testClientInfo("Owner", "desktop", 1)); err != nil {
		t.Fatal(err)
	}
	info := testClientInfo("New Mac", "desktop", 2)
	pending, _, err := service.CreatePairing(info)
	if err != nil {
		t.Fatal(err)
	}

	approved, err := service.RegisterApproved(info)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Token == "" || approved.Pairing != nil || approved.Device.Status != storage.DeviceStatusApproved {
		t.Fatalf("approved registration = %#v", approved)
	}
	if _, err := service.Authenticate(approved.Token, SeenInfo{}); err != nil {
		t.Fatalf("approved token auth: %v", err)
	}

	_, pairings, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pairing := range pairings {
		if pairing.ID != pending.ID {
			continue
		}
		found = true
		if pairing.Status != storage.PairingStatusRejected {
			t.Fatalf("pending pairing status = %q, want rejected", pairing.Status)
		}
	}
	if !found {
		t.Fatalf("pending pairing %s not found", pending.ID)
	}
}

func TestCreatePairingReplacesPendingRequestForDevice(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := New(store)
	info := testClientInfo("Mac", "desktop", 1)
	if _, err := service.Register(info); err != nil {
		t.Fatal(err)
	}
	first, _, err := service.CreatePairing(info)
	if err != nil {
		t.Fatal(err)
	}
	second, secret, err := service.CreatePairing(info)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("pairing id reused: %s", first.ID)
	}

	_, pairings, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, pairing := range pairings {
		statuses[pairing.ID] = pairing.Status
	}
	if statuses[first.ID] != storage.PairingStatusRejected {
		t.Fatalf("first status = %q, want rejected", statuses[first.ID])
	}
	if statuses[second.ID] != storage.PairingStatusPending {
		t.Fatalf("second status = %q, want pending", statuses[second.ID])
	}
	if _, err := service.ApprovePairing(first.ID); err == nil {
		t.Fatal("approved replaced pairing")
	}
	if _, err := service.ApprovePairing(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(secret, SeenInfo{}); err != nil {
		t.Fatalf("replacement token auth: %v", err)
	}
}

func testClientInfo(name, kind string, seed byte) Registration {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed
	}
	sum := sha256.Sum256(raw)
	return Registration{
		Identity: DeviceIdentity{
			DeviceID:  hex.EncodeToString(sum[:]),
			PublicKey: base64.RawURLEncoding.EncodeToString(raw),
		},
		Profile: DeviceProfile{
			Name:       name,
			Kind:       kind,
			Platform:   "macOS",
			Family:     "Mac",
			Model:      "MacBookPro18,3",
			AppVersion: "0.1.0",
		},
	}
}
