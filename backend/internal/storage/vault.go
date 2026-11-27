package storage

import (
	"context"
	"errors"
	"time"
)

const (
	VaultKindLogin    = "login"
	VaultKindCard     = "card"
	VaultKindIdentity = "identity"
)

var ErrVaultItemNotFound = errors.New("vault item not found")

// VaultItem holds non-secret metadata safe for UI listing and LLM discovery.
// Raw secrets live exclusively in VaultSecretPayload and are never serialized here.
type VaultItem struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Label          string    `json:"label"`
	Origins        []string  `json:"origins,omitempty"`
	Username       string    `json:"username,omitempty"`
	HasTOTP        bool      `json:"has_totp,omitempty"`
	CardBrand      string    `json:"card_brand,omitempty"`
	CardLast4      string    `json:"card_last4,omitempty"`
	CardExpMonth   int       `json:"card_exp_month,omitempty"`
	CardExpYear    int       `json:"card_exp_year,omitempty"`
	CardholderName string    `json:"cardholder_name,omitempty"`
	BillingSummary string    `json:"billing_summary,omitempty"`
	Email          string    `json:"email,omitempty"`
	CityCountry    string    `json:"city_country,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// VaultSecretPayload is encrypted at rest and decrypted only inside the trusted
// Vault broker immediately prior to browser injection.
type VaultSecretPayload struct {
	Password       string `json:"password,omitempty"`
	TOTPSecret     string `json:"totp_secret,omitempty"`
	PAN            string `json:"pan,omitempty"`
	CVV            string `json:"cvv,omitempty"`
	BillingAddress string `json:"billing_address,omitempty"`
	FullName       string `json:"full_name,omitempty"`
	Phone          string `json:"phone,omitempty"`
	AddressLines   string `json:"address_lines,omitempty"`
	PostalCode     string `json:"postal_code,omitempty"`
	Country        string `json:"country,omitempty"`
}

type VaultItemRecord struct {
	Item   VaultItem
	Secret VaultSecretPayload
}

type VaultStore interface {
	ListVaultItems(ctx context.Context, kind string) ([]VaultItem, error)
	GetVaultItem(ctx context.Context, id string) (VaultItem, error)
	GetVaultItemRecord(ctx context.Context, id string) (VaultItemRecord, error)
	CreateVaultItem(ctx context.Context, record VaultItemRecord) (VaultItem, error)
	UpdateVaultItem(ctx context.Context, record VaultItemRecord) (VaultItem, error)
	DeleteVaultItem(ctx context.Context, id string) error
}
