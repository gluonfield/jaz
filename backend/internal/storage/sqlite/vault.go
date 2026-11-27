package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	vaultdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/vault"
)

type vaultMetaJSON struct {
	Username       string `json:"username,omitempty"`
	HasTOTP        bool   `json:"has_totp,omitempty"`
	CardBrand      string `json:"card_brand,omitempty"`
	CardLast4      string `json:"card_last4,omitempty"`
	CardExpMonth   int    `json:"card_exp_month,omitempty"`
	CardExpYear    int    `json:"card_exp_year,omitempty"`
	CardholderName string `json:"cardholder_name,omitempty"`
	BillingSummary string `json:"billing_summary,omitempty"`
	Email          string `json:"email,omitempty"`
	CityCountry    string `json:"city_country,omitempty"`
}

func vaultSecretID(id string) string {
	return "vault:" + id
}

func encodeVaultMeta(item storage.VaultItem) string {
	return marshalJSONString(vaultMetaJSON{
		Username:       item.Username,
		HasTOTP:        item.HasTOTP,
		CardBrand:      item.CardBrand,
		CardLast4:      item.CardLast4,
		CardExpMonth:   item.CardExpMonth,
		CardExpYear:    item.CardExpYear,
		CardholderName: item.CardholderName,
		BillingSummary: item.BillingSummary,
		Email:          item.Email,
		CityCountry:    item.CityCountry,
	}, "{}")
}

func vaultItemFromColumns(id, kind, label, originsJSON, metaJSON string, createdAtMs, updatedAtMs int64) storage.VaultItem {
	var meta vaultMetaJSON
	_ = json.Unmarshal([]byte(metaJSON), &meta)
	origins := unmarshalStringSlice(originsJSON)
	if len(origins) == 0 {
		origins = nil
	}
	return storage.VaultItem{
		ID:             id,
		Kind:           kind,
		Label:          label,
		Origins:        origins,
		Username:       meta.Username,
		HasTOTP:        meta.HasTOTP,
		CardBrand:      meta.CardBrand,
		CardLast4:      meta.CardLast4,
		CardExpMonth:   meta.CardExpMonth,
		CardExpYear:    meta.CardExpYear,
		CardholderName: meta.CardholderName,
		BillingSummary: meta.BillingSummary,
		Email:          meta.Email,
		CityCountry:    meta.CityCountry,
		CreatedAt:      msToTime(createdAtMs),
		UpdatedAt:      msToTime(updatedAtMs),
	}
}

func (s *Store) encryptVaultSecret(id string, secret storage.VaultSecretPayload) (string, error) {
	raw, err := json.Marshal(secret)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	return s.encryptSecret(vaultSecretID(id), raw)
}

func (s *Store) decryptVaultSecret(id, ciphertext string) (storage.VaultSecretPayload, error) {
	plain, err := s.decryptSecret(vaultSecretID(id), ciphertext)
	if err != nil {
		return storage.VaultSecretPayload{}, err
	}
	defer clear(plain)
	var secret storage.VaultSecretPayload
	if err := json.Unmarshal(plain, &secret); err != nil {
		return storage.VaultSecretPayload{}, err
	}
	return secret, nil
}

func (s *Store) ListVaultItems(ctx context.Context, kind string) ([]storage.VaultItem, error) {
	rows, err := vaultdb.New(s.db).ListVaultItems(ctx, strings.TrimSpace(kind))
	if err != nil {
		return nil, err
	}
	out := make([]storage.VaultItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, vaultItemFromColumns(row.ID, row.Kind, row.Label, row.OriginsJson, row.MetaJson, row.CreatedAtMs, row.UpdatedAtMs))
	}
	return out, nil
}

func (s *Store) GetVaultItem(ctx context.Context, id string) (storage.VaultItem, error) {
	row, err := vaultdb.New(s.db).GetVaultItemMeta(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.VaultItem{}, storage.ErrVaultItemNotFound
	}
	if err != nil {
		return storage.VaultItem{}, err
	}
	return vaultItemFromColumns(row.ID, row.Kind, row.Label, row.OriginsJson, row.MetaJson, row.CreatedAtMs, row.UpdatedAtMs), nil
}

func (s *Store) GetVaultItemRecord(ctx context.Context, id string) (storage.VaultItemRecord, error) {
	row, err := vaultdb.New(s.db).GetVaultItemRecord(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.VaultItemRecord{}, storage.ErrVaultItemNotFound
	}
	if err != nil {
		return storage.VaultItemRecord{}, err
	}
	secret, err := s.decryptVaultSecret(row.ID, row.SecretEncrypted)
	if err != nil {
		return storage.VaultItemRecord{}, err
	}
	return storage.VaultItemRecord{
		Item:   vaultItemFromColumns(row.ID, row.Kind, row.Label, row.OriginsJson, row.MetaJson, row.CreatedAtMs, row.UpdatedAtMs),
		Secret: secret,
	}, nil
}

func (s *Store) CreateVaultItem(ctx context.Context, record storage.VaultItemRecord) (storage.VaultItem, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC()
	if record.Item.CreatedAt.IsZero() {
		record.Item.CreatedAt = now
	}
	if record.Item.UpdatedAt.IsZero() {
		record.Item.UpdatedAt = record.Item.CreatedAt
	}
	sealed, err := s.encryptVaultSecret(record.Item.ID, record.Secret)
	if err != nil {
		return storage.VaultItem{}, err
	}
	if err := vaultdb.New(s.db).InsertVaultItem(ctx, vaultdb.InsertVaultItemParams{
		ID:              record.Item.ID,
		Kind:            record.Item.Kind,
		Label:           record.Item.Label,
		OriginsJson:     marshalJSONString(record.Item.Origins, "[]"),
		MetaJson:        encodeVaultMeta(record.Item),
		SecretEncrypted: sealed,
		CreatedAtMs:     timeToMs(record.Item.CreatedAt),
		UpdatedAtMs:     timeToMs(record.Item.UpdatedAt),
	}); err != nil {
		return storage.VaultItem{}, err
	}
	return record.Item, nil
}

func (s *Store) UpdateVaultItem(ctx context.Context, record storage.VaultItemRecord) (storage.VaultItem, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if record.Item.UpdatedAt.IsZero() {
		record.Item.UpdatedAt = time.Now().UTC()
	}
	sealed, err := s.encryptVaultSecret(record.Item.ID, record.Secret)
	if err != nil {
		return storage.VaultItem{}, err
	}
	affected, err := vaultdb.New(s.db).UpdateVaultItem(ctx, vaultdb.UpdateVaultItemParams{
		Label:           record.Item.Label,
		OriginsJson:     marshalJSONString(record.Item.Origins, "[]"),
		MetaJson:        encodeVaultMeta(record.Item),
		SecretEncrypted: sealed,
		UpdatedAtMs:     timeToMs(record.Item.UpdatedAt),
		ID:              record.Item.ID,
	})
	if err != nil {
		return storage.VaultItem{}, err
	}
	if affected == 0 {
		return storage.VaultItem{}, storage.ErrVaultItemNotFound
	}
	return record.Item, nil
}

func (s *Store) DeleteVaultItem(ctx context.Context, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	affected, err := vaultdb.New(s.db).DeleteVaultItem(ctx, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return storage.ErrVaultItemNotFound
	}
	return nil
}
