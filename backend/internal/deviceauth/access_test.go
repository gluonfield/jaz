package deviceauth

import (
	"testing"
	"time"

	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestPairingProofIsSingleUseAndTicketsExpire(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := New(store)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	pairing, secret, err := service.CreatePairing(testClientInfo("Phone", "mobile", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApprovePairing(pairing.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(secret, SeenInfo{}); err == nil {
		t.Fatal("pairing proof authenticated before credential exchange")
	}
	poll, err := service.PollPairing(pairing.ID, secret)
	if err != nil || poll.Token == "" || poll.Token == secret {
		t.Fatalf("separate credential was not issued: %v", err)
	}
	if _, err := service.PollPairing(pairing.ID, secret); err == nil {
		t.Fatal("pairing proof could be redeemed twice")
	}
	if _, err := service.Authenticate(secret, SeenInfo{}); err == nil {
		t.Fatal("pairing proof authenticated as a device")
	}
	ticket, err := service.IssueResourceTicket(poll.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateResourceTicket(ticket.Token); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := service.AuthenticateResourceTicket(ticket.Token); err == nil {
		t.Fatal("expired ticket accepted")
	}
	if _, err := service.Authenticate(poll.Token, SeenInfo{}); err != nil {
		t.Fatalf("ticket expiry invalidated the device: %v", err)
	}
	pairing, _, err = service.CreatePairing(testClientInfo("Other phone", "mobile", 2))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if _, err := service.ApprovePairing(pairing.ID); err == nil {
		t.Fatal("expired pairing approved")
	}
}

func TestEnrollmentInvitationsExpireAndAreRevokedWithTheirOwner(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := New(store)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	owner, err := service.RegisterApproved(testClientInfo("Owner", "desktop", 1))
	if err != nil {
		t.Fatal(err)
	}
	invite, err := service.CreateInvitation(owner.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := service.RegisterInvited(invite, testClientInfo("Phone", "mobile", 2))
	if err != nil || registered.Token == "" {
		t.Fatalf("invitation registration failed: %v", err)
	}
	if _, err := service.RegisterInvited(invite, testClientInfo("Other phone", "mobile", 3)); err == nil {
		t.Fatal("invitation reused")
	}
	invite, err = service.CreateInvitation(owner.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if _, err := service.RegisterInvited(invite, testClientInfo("Other phone", "mobile", 3)); err == nil {
		t.Fatal("expired invitation reused")
	}
	invite, err = service.CreateInvitation(owner.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeDevice(owner.Device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateInvitation(owner.Device.ID); err == nil {
		t.Fatal("revoked owner could create another invitation")
	}
	if service.IsInvitation(invite) {
		t.Fatal("revoked device retained enrollment authority")
	}
}
