package deviceauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

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
