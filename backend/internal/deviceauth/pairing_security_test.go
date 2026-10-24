package deviceauth

import (
	"sync"
	"testing"
	"time"

	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestPairingCannotEditApprovedProfileBeforeClaim(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	service := New(store)
	info := testClientInfo("Trusted phone", "mobile", 1)
	registered, err := service.RegisterApproved(info)
	if err != nil {
		t.Fatal(err)
	}
	proposal := info
	proposal.Profile.Name = "Untrusted replacement"
	proposal.Profile.Kind = "desktop"
	pairing, proof, err := service.CreatePairing(proposal)
	if err != nil {
		t.Fatal(err)
	}
	for _, approve := range []bool{false, true} {
		if approve {
			if _, err := service.ApprovePairing(pairing.ID); err != nil {
				t.Fatal(err)
			}
		}
		device, err := store.LoadDevice(registered.Device.ID)
		if err != nil {
			t.Fatal(err)
		}
		if device.Name != info.Profile.Name || device.Kind != info.Profile.Kind {
			t.Fatalf("unclaimed proposal changed trusted profile: %+v", device)
		}
		if _, err := service.Authenticate(registered.Token, SeenInfo{}); err != nil {
			t.Fatalf("unclaimed proposal changed credential: %v", err)
		}
	}
	poll, err := service.PollPairing(pairing.ID, proof)
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.LoadDevice(registered.Device.ID)
	if err != nil || device.Name != proposal.Profile.Name || device.Kind != proposal.Profile.Kind {
		t.Fatalf("approved exchange did not apply profile: %+v, %v", device, err)
	}
	if _, err := service.Authenticate(poll.Token, SeenInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(registered.Token, SeenInfo{}); err == nil {
		t.Fatal("old credential survived completed exchange")
	}
}

func TestConcurrentPairingAdmissionLeavesNoExtraDevices(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	service := New(store)
	now := time.Now().UTC()
	service.now = func() time.Time {
		return now
	}
	var pending sync.WaitGroup
	accepted := make(chan bool, 20)
	for seed := byte(1); seed <= 20; seed++ {
		pending.Go(func() {
			_, _, err := service.CreatePairing(testClientInfo("Phone", "mobile", seed))
			accepted <- err == nil
		})
	}
	pending.Wait()
	close(accepted)
	count := 0
	for ok := range accepted {
		if ok {
			count++
		}
	}
	devices, pairings, err := service.List()
	if err != nil || count != 5 || len(devices) != 5 || len(pairings) != 5 {
		t.Fatalf("concurrent admission exceeded its transaction bound: accepted=%d devices=%d pairings=%d error=%v", count, len(devices), len(pairings), err)
	}
}

func TestPendingPairingCapAndExpiry(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	service := New(store)
	now := time.Now().UTC()
	service.now = func() time.Time {
		return now
	}
	for seed := byte(1); seed <= 32; seed++ {
		if seed > 1 && (seed-1)%5 == 0 {
			now = now.Add(time.Minute)
		}
		if _, _, err := service.CreatePairing(testClientInfo("Phone", "mobile", seed)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := service.CreatePairing(testClientInfo("Excess phone", "mobile", 33)); err == nil {
		t.Fatal("pending pairing cap was exceeded")
	}
	now = now.Add(11 * time.Minute)
	_, pairings, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, pairing := range pairings {
		if pairing.Status != "expired" {
			t.Fatal("expired requests still appear actionable")
		}
	}
	if _, _, err := service.CreatePairing(testClientInfo("Later phone", "mobile", 34)); err != nil {
		t.Fatalf("expired requests still occupy admission slots: %v", err)
	}
}

func TestOldPairingProofCannotOverrideReenrollment(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "rotation", true: "revocation"}[revoke], func(t *testing.T) {
			store, err := sqlite.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = store.Close()
			})
			service := New(store)
			info := testClientInfo("Phone", "mobile", 1)
			registered, err := service.RegisterApproved(info)
			if err != nil {
				t.Fatal(err)
			}
			pairing, proof, err := service.CreatePairing(info)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ApprovePairing(pairing.ID); err != nil {
				t.Fatal(err)
			}
			if revoke {
				if _, err := service.RevokeDevice(registered.Device.ID); err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := service.RegisterApproved(info)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.PollPairing(pairing.ID, proof); err == nil {
				t.Fatal("older authorization replaced the new credential")
			}
			if _, err := service.Authenticate(fresh.Token, SeenInfo{}); err != nil {
				t.Fatalf("fresh credential invalidated: %v", err)
			}
		})
	}
}

func TestPairingAdmissionIsBoundedAndRecoversAfterWindow(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	service := New(store)
	now := time.Now().UTC()
	service.now = func() time.Time {
		return now
	}
	for seed := byte(1); seed <= 5; seed++ {
		if _, _, err := service.CreatePairing(testClientInfo("Phone", "mobile", seed)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := service.CreatePairing(testClientInfo("Excess request", "mobile", 6)); err == nil {
		t.Fatal("pairing admission did not rate limit")
	}
	now = now.Add(time.Minute)
	if _, _, err := service.CreatePairing(testClientInfo("Later phone", "mobile", 7)); err != nil {
		t.Fatalf("admission failed to recover: %v", err)
	}
	now = now.Add(25 * time.Hour)
	if _, _, err := service.CreatePairing(testClientInfo("Next day", "mobile", 8)); err != nil {
		t.Fatal(err)
	}
	devices, pairings, err := service.List()
	if err != nil || len(devices) != 1 || len(pairings) != 1 {
		t.Fatalf("expired anonymous records retained: devices=%d pairings=%d error=%v", len(devices), len(pairings), err)
	}
}
