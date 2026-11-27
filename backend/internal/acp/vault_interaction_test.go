package acp

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestVaultApprovalAndRequestPermissionRoundTrip(t *testing.T) {
	manager, _, session, ctx := askUserFixture(t)
	events := manager.Events.Subscribe(ctx, session.ID)

	// 1. Card approval approved once
	approveCh := make(chan bool, 1)
	errCh := make(chan error, 1)
	go func() {
		approved, err := manager.ConfirmVaultFill(ctx, session.ID, sessionevents.VaultApprovalSpec{
			ItemID:          "vlt_card_1",
			Kind:            "card",
			Label:           "Personal Visa",
			CardBrand:       "Visa",
			CardLast4:       "4242",
			MerchantOrigin:  "https://checkout.example.com",
			CheckoutSummary: "£25.00",
		})
		if err != nil {
			errCh <- err
			return
		}
		approveCh <- approved
	}()

	var permID string
	for ev := range events {
		if ev.Type == "permission_request" && ev.Permission != nil && ev.Permission.VaultApproval != nil {
			permID = ev.Permission.ID
			break
		}
	}
	if permID == "" {
		t.Fatal("did not receive vault approval permission_request event")
	}
	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   session.ID,
		RequestID: permID,
		OptionID:  vaultApproveOptionID,
	}); err != nil {
		t.Fatalf("AnswerInteractive approve: %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("ConfirmVaultFill failed: %v", err)
	case approved := <-approveCh:
		if !approved {
			t.Fatal("expected approved=true")
		}
	}

	// 2. RequestVaultItem accepts opaque saved:<itemID> and rejects bogus option IDs
	type reqResult struct {
		itemID    string
		cancelled bool
		err       error
	}
	reqCh := make(chan reqResult, 1)
	go func() {
		id, cancelled, err := manager.RequestVaultItem(ctx, session.ID, sessionevents.VaultRequestSpec{
			Kind:   "login",
			Domain: "https://github.com",
			Label:  "GitHub",
		})
		reqCh <- reqResult{itemID: id, cancelled: cancelled, err: err}
	}()

	var reqPermID string
	for ev := range events {
		if ev.Type == "permission_request" && ev.Permission != nil && ev.Permission.VaultRequest != nil {
			reqPermID = ev.Permission.ID
			break
		}
	}
	if reqPermID == "" {
		t.Fatal("did not receive vault request permission_request event")
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   session.ID,
		RequestID: reqPermID,
		OptionID:  "invalid_option",
	}); err == nil {
		t.Fatal("expected unknown option rejected for VaultRequest")
	}

	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{
		Session:   session.ID,
		RequestID: reqPermID,
		OptionID:  "saved:vlt_new_123",
	}); err != nil {
		t.Fatalf("AnswerInteractive saved handle: %v", err)
	}

	res := <-reqCh
	if res.err != nil || res.cancelled || res.itemID != "vlt_new_123" {
		t.Fatalf("unexpected RequestVaultItem result: %+v", res)
	}
}
