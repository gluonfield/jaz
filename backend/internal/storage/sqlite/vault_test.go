package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestVaultStoreCRUDAndEncryptionAtRest(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	loginRec := storage.VaultItemRecord{
		Item: storage.VaultItem{
			ID:        "vlt_login_1",
			Kind:      storage.VaultKindLogin,
			Label:     "GitHub",
			Origins:   []string{"https://github.com"},
			Username:  "octocat",
			HasTOTP:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Secret: storage.VaultSecretPayload{
			Password:   "super-secret-password-99",
			TOTPSecret: "JBSWY3DPEHPK3PXP",
		},
	}

	created, err := store.CreateVaultItem(ctx, loginRec)
	if err != nil {
		t.Fatalf("CreateVaultItem: %v", err)
	}
	if created.ID != loginRec.Item.ID || created.Username != "octocat" || !created.HasTOTP {
		t.Fatalf("unexpected created metadata: %+v", created)
	}

	var rawEncrypted string
	if err := store.db.QueryRow(`SELECT secret_encrypted FROM vault_items WHERE id = ?`, loginRec.Item.ID).Scan(&rawEncrypted); err != nil {
		t.Fatalf("read raw encrypted column: %v", err)
	}
	if !strings.HasPrefix(rawEncrypted, secretPrefix) {
		t.Fatalf("expected encrypted prefix %q, got %q", secretPrefix, rawEncrypted)
	}
	if strings.Contains(rawEncrypted, "super-secret-password-99") || strings.Contains(rawEncrypted, "JBSWY3DPEHPK3PXP") {
		t.Fatalf("raw vault_items.secret_encrypted leaked plaintext secret")
	}

	cardRec := storage.VaultItemRecord{
		Item: storage.VaultItem{
			ID:             "vlt_card_1",
			Kind:           storage.VaultKindCard,
			Label:          "Personal Visa",
			CardBrand:      "Visa",
			CardLast4:      "4242",
			CardExpMonth:   12,
			CardExpYear:    2030,
			CardholderName: "Jane Doe",
			CreatedAt:      now.Add(time.Second),
			UpdatedAt:      now.Add(time.Second),
		},
		Secret: storage.VaultSecretPayload{
			PAN: "4242424242424242",
			CVV: "842",
		},
	}
	if _, err := store.CreateVaultItem(ctx, cardRec); err != nil {
		t.Fatalf("CreateVaultItem card: %v", err)
	}

	all, err := store.ListVaultItems(ctx, "")
	if err != nil {
		t.Fatalf("ListVaultItems all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 items, got %d", len(all))
	}
	cards, err := store.ListVaultItems(ctx, storage.VaultKindCard)
	if err != nil {
		t.Fatalf("ListVaultItems card: %v", err)
	}
	if len(cards) != 1 || cards[0].ID != "vlt_card_1" {
		t.Fatalf("unexpected card list: %+v", cards)
	}

	loaded, err := store.GetVaultItemRecord(ctx, loginRec.Item.ID)
	if err != nil {
		t.Fatalf("GetVaultItemRecord: %v", err)
	}
	if loaded.Secret.Password != "super-secret-password-99" || loaded.Secret.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypted secret mismatch: %+v", loaded.Secret)
	}

	loaded.Item.Label = "GitHub Work"
	loaded.Secret.Password = "rotated-secret-42"
	loaded.Item.UpdatedAt = now.Add(2 * time.Second)
	updated, err := store.UpdateVaultItem(ctx, loaded)
	if err != nil {
		t.Fatalf("UpdateVaultItem: %v", err)
	}
	if updated.Label != "GitHub Work" {
		t.Fatalf("expected updated label, got %q", updated.Label)
	}
	reloaded, err := store.GetVaultItemRecord(ctx, loginRec.Item.ID)
	if err != nil {
		t.Fatalf("GetVaultItemRecord after update: %v", err)
	}
	if reloaded.Secret.Password != "rotated-secret-42" {
		t.Fatalf("expected rotated password, got %q", reloaded.Secret.Password)
	}

	if err := store.DeleteVaultItem(ctx, loginRec.Item.ID); err != nil {
		t.Fatalf("DeleteVaultItem: %v", err)
	}
	if err := store.DeleteVaultItem(ctx, loginRec.Item.ID); !errors.Is(err, storage.ErrVaultItemNotFound) {
		t.Fatalf("expected ErrVaultItemNotFound on second delete, got %v", err)
	}
	if _, err := store.GetVaultItem(ctx, loginRec.Item.ID); !errors.Is(err, storage.ErrVaultItemNotFound) {
		t.Fatalf("expected ErrVaultItemNotFound after delete, got %v", err)
	}
}
