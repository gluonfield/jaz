package vault

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type Service struct {
	store storage.VaultStore
	now   func() time.Time
}

func NewService(store storage.VaultStore) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) List(ctx context.Context, kind, domain string) ([]storage.VaultItem, error) {
	if kind != "" {
		normalized, err := NormalizeKind(kind)
		if err != nil {
			return nil, err
		}
		kind = normalized
	}
	items, err := s.store.ListVaultItems(ctx, kind)
	if err != nil {
		return nil, err
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		if items == nil {
			return []storage.VaultItem{}, nil
		}
		return items, nil
	}
	filtered := make([]storage.VaultItem, 0, len(items))
	for _, item := range items {
		if item.Kind == storage.VaultKindLogin && !DomainMatchesFilter(item.Origins, domain) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *Service) Get(ctx context.Context, id string) (storage.VaultItem, error) {
	return s.store.GetVaultItem(ctx, strings.TrimSpace(id))
}

func (s *Service) Create(ctx context.Context, input CreateInput) (storage.VaultItem, error) {
	kind, err := NormalizeKind(input.Kind)
	if err != nil {
		return storage.VaultItem{}, err
	}
	now := s.now()
	record := storage.VaultItemRecord{
		Item: storage.VaultItem{
			ID:        newVaultID(),
			Kind:      kind,
			Label:     strings.TrimSpace(input.Label),
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	if err := s.applyCreateFields(&record, input); err != nil {
		return storage.VaultItem{}, err
	}
	return s.store.CreateVaultItem(ctx, record)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (storage.VaultItem, error) {
	record, err := s.store.GetVaultItemRecord(ctx, strings.TrimSpace(id))
	if err != nil {
		return storage.VaultItem{}, err
	}
	if input.Label != nil {
		record.Item.Label = strings.TrimSpace(*input.Label)
	}
	switch record.Item.Kind {
	case storage.VaultKindLogin:
		if input.ReplaceOrigins || len(input.Origins) > 0 {
			if err := setLoginOrigins(&record, input.Origins); err != nil {
				return storage.VaultItem{}, err
			}
		}
		if input.Username != nil {
			record.Item.Username = strings.TrimSpace(*input.Username)
		}
		if input.Password != nil && *input.Password != "" {
			record.Secret.Password = *input.Password
		}
		if input.TOTPSecret != nil {
			if err := s.setLoginTOTP(&record, *input.TOTPSecret); err != nil {
				return storage.VaultItem{}, err
			}
		}
	case storage.VaultKindCard:
		if input.CardholderName != nil {
			record.Item.CardholderName = strings.TrimSpace(*input.CardholderName)
		}
		if input.PAN != nil && strings.TrimSpace(*input.PAN) != "" {
			if err := setCardPAN(&record, *input.PAN); err != nil {
				return storage.VaultItem{}, err
			}
		}
		if input.CardExpMonth != nil {
			if err := setCardExpMonth(&record, *input.CardExpMonth); err != nil {
				return storage.VaultItem{}, err
			}
		}
		if input.CardExpYear != nil {
			if err := setCardExpYear(&record, *input.CardExpYear); err != nil {
				return storage.VaultItem{}, err
			}
		}
		if input.CVV != nil && strings.TrimSpace(*input.CVV) != "" {
			if err := setCardCVV(&record, *input.CVV); err != nil {
				return storage.VaultItem{}, err
			}
		}
		if input.BillingAddress != nil {
			record.Secret.BillingAddress = strings.TrimSpace(*input.BillingAddress)
			record.Item.BillingSummary = SummarizeBillingAddress(record.Secret.BillingAddress, valueOrEmpty(input.BillingSummary))
		} else if input.BillingSummary != nil {
			record.Item.BillingSummary = strings.TrimSpace(*input.BillingSummary)
		}
	case storage.VaultKindIdentity:
		if input.Email != nil {
			record.Item.Email = strings.TrimSpace(*input.Email)
		}
		if input.FullName != nil {
			record.Secret.FullName = strings.TrimSpace(*input.FullName)
		}
		if input.Phone != nil {
			record.Secret.Phone = strings.TrimSpace(*input.Phone)
		}
		if input.AddressLines != nil {
			record.Secret.AddressLines = strings.TrimSpace(*input.AddressLines)
		}
		if input.PostalCode != nil {
			record.Secret.PostalCode = strings.TrimSpace(*input.PostalCode)
		}
		if input.Country != nil {
			record.Secret.Country = strings.TrimSpace(*input.Country)
		}
		if input.CityCountry != nil {
			record.Item.CityCountry = strings.TrimSpace(*input.CityCountry)
		}
	}
	if record.Item.Label == "" {
		record.Item.Label = defaultLabel(record.Item)
	}
	record.Item.UpdatedAt = s.now()
	return s.store.UpdateVaultItem(ctx, record)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteVaultItem(ctx, strings.TrimSpace(id))
}

// ResolveFillValues decrypts only the requested slots for the trusted browser host.
// For logins it strictly enforces exact origin match against expectedOrigin.
func (s *Service) ResolveFillValues(ctx context.Context, id, expectedOrigin string, slots []string) (storage.VaultItem, map[string]string, error) {
	record, err := s.store.GetVaultItemRecord(ctx, strings.TrimSpace(id))
	if err != nil {
		return storage.VaultItem{}, nil, err
	}
	if record.Item.Kind == storage.VaultKindLogin && !OriginMatchesLogin(record.Item.Origins, expectedOrigin) {
		return storage.VaultItem{}, nil, fmt.Errorf("%w: %s is not allowed for login %s", ErrOriginMismatch, expectedOrigin, record.Item.ID)
	}
	values := make(map[string]string, len(slots))
	for _, rawSlot := range slots {
		slot := strings.ToLower(strings.TrimSpace(rawSlot))
		val, err := s.slotValue(record, slot)
		if err != nil {
			return storage.VaultItem{}, nil, err
		}
		values[slot] = val
	}
	return record.Item, values, nil
}

func (s *Service) slotValue(record storage.VaultItemRecord, slot string) (string, error) {
	switch record.Item.Kind {
	case storage.VaultKindLogin:
		switch slot {
		case "username":
			return record.Item.Username, nil
		case "password":
			if record.Secret.Password == "" {
				return "", fmt.Errorf("%w: login has no password stored", ErrInvalidInput)
			}
			return record.Secret.Password, nil
		case "totp":
			if record.Secret.TOTPSecret == "" {
				return "", fmt.Errorf("%w: login has no TOTP secret stored", ErrInvalidInput)
			}
			return GenerateTOTP(record.Secret.TOTPSecret, s.now())
		default:
			return "", fmt.Errorf("%w: slot %q is not valid for login items", ErrInvalidInput, slot)
		}
	case storage.VaultKindCard:
		switch slot {
		case "cc_number":
			return record.Secret.PAN, nil
		case "cc_cvv":
			return record.Secret.CVV, nil
		case "cc_exp_month":
			return fmt.Sprintf("%02d", record.Item.CardExpMonth), nil
		case "cc_exp_year":
			return fmt.Sprintf("%04d", record.Item.CardExpYear), nil
		case "cc_exp_mmyy":
			return fmt.Sprintf("%02d/%02d", record.Item.CardExpMonth, record.Item.CardExpYear%100), nil
		case "cc_name":
			return record.Item.CardholderName, nil
		case "billing_address":
			return record.Secret.BillingAddress, nil
		default:
			return "", fmt.Errorf("%w: slot %q is not valid for card items", ErrInvalidInput, slot)
		}
	case storage.VaultKindIdentity:
		switch slot {
		case "email":
			return record.Item.Email, nil
		case "full_name":
			return record.Secret.FullName, nil
		case "phone":
			return record.Secret.Phone, nil
		case "address_lines":
			return record.Secret.AddressLines, nil
		case "postal_code":
			return record.Secret.PostalCode, nil
		case "country":
			return record.Secret.Country, nil
		default:
			return "", fmt.Errorf("%w: slot %q is not valid for identity items", ErrInvalidInput, slot)
		}
	default:
		return "", fmt.Errorf("%w: unsupported vault kind %q", ErrInvalidInput, record.Item.Kind)
	}
}

func (s *Service) applyCreateFields(record *storage.VaultItemRecord, input CreateInput) error {
	switch record.Item.Kind {
	case storage.VaultKindLogin:
		if err := setLoginOrigins(record, input.Origins); err != nil {
			return err
		}
		if input.Password == "" {
			return fmt.Errorf("%w: login requires a password", ErrInvalidInput)
		}
		if err := s.setLoginTOTP(record, input.TOTPSecret); err != nil {
			return err
		}
		record.Item.Username = strings.TrimSpace(input.Username)
		record.Secret.Password = input.Password
	case storage.VaultKindCard:
		if err := setCardPAN(record, input.PAN); err != nil {
			return err
		}
		if err := setCardCVV(record, input.CVV); err != nil {
			return err
		}
		if err := setCardExpMonth(record, input.CardExpMonth); err != nil {
			return err
		}
		if err := setCardExpYear(record, input.CardExpYear); err != nil {
			return err
		}
		record.Item.CardholderName = strings.TrimSpace(input.CardholderName)
		record.Secret.BillingAddress = strings.TrimSpace(input.BillingAddress)
		record.Item.BillingSummary = SummarizeBillingAddress(record.Secret.BillingAddress, input.BillingSummary)
	case storage.VaultKindIdentity:
		email := strings.TrimSpace(input.Email)
		fullName := strings.TrimSpace(input.FullName)
		if email == "" && fullName == "" {
			return fmt.Errorf("%w: identity requires full_name or email", ErrInvalidInput)
		}
		record.Item.Email = email
		record.Item.CityCountry = strings.TrimSpace(input.CityCountry)
		if record.Item.CityCountry == "" {
			record.Item.CityCountry = strings.TrimSpace(input.Country)
		}
		record.Secret.FullName = fullName
		record.Secret.Phone = strings.TrimSpace(input.Phone)
		record.Secret.AddressLines = strings.TrimSpace(input.AddressLines)
		record.Secret.PostalCode = strings.TrimSpace(input.PostalCode)
		record.Secret.Country = strings.TrimSpace(input.Country)
	}
	if record.Item.Label == "" {
		record.Item.Label = defaultLabel(record.Item)
	}
	return nil
}

func setLoginOrigins(record *storage.VaultItemRecord, rawOrigins []string) error {
	origins, err := NormalizeOrigins(rawOrigins)
	if err != nil {
		return err
	}
	if len(origins) == 0 {
		return fmt.Errorf("%w: login requires at least one origin", ErrInvalidInput)
	}
	record.Item.Origins = origins
	return nil
}

func (s *Service) setLoginTOTP(record *storage.VaultItemRecord, rawSecret string) error {
	secret := strings.TrimSpace(rawSecret)
	if secret != "" {
		if _, err := GenerateTOTP(secret, s.now()); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}
	record.Secret.TOTPSecret = secret
	record.Item.HasTOTP = secret != ""
	return nil
}

func setCardPAN(record *storage.VaultItemRecord, raw string) error {
	pan := NormalizeDigits(raw)
	if len(pan) < 12 || len(pan) > 19 {
		return fmt.Errorf("%w: card number must be 12 to 19 digits", ErrInvalidInput)
	}
	record.Secret.PAN = pan
	record.Item.CardBrand = DetectCardBrand(pan)
	record.Item.CardLast4 = pan[len(pan)-4:]
	return nil
}

func setCardCVV(record *storage.VaultItemRecord, raw string) error {
	cvv := NormalizeDigits(raw)
	if len(cvv) < 3 || len(cvv) > 4 {
		return fmt.Errorf("%w: card CVV must be 3 or 4 digits", ErrInvalidInput)
	}
	record.Secret.CVV = cvv
	return nil
}

func setCardExpMonth(record *storage.VaultItemRecord, month int) error {
	if month < 1 || month > 12 {
		return fmt.Errorf("%w: card expiration month must be 1-12", ErrInvalidInput)
	}
	record.Item.CardExpMonth = month
	return nil
}

func setCardExpYear(record *storage.VaultItemRecord, rawYear int) error {
	year := normalizeExpYear(rawYear)
	if year < 2000 || year > 2100 {
		return fmt.Errorf("%w: card expiration year is invalid", ErrInvalidInput)
	}
	record.Item.CardExpYear = year
	return nil
}

func defaultLabel(item storage.VaultItem) string {
	switch item.Kind {
	case storage.VaultKindLogin:
		if len(item.Origins) > 0 {
			if u, err := url.Parse(item.Origins[0]); err == nil && u.Host != "" {
				return u.Host
			}
		}
		if item.Username != "" {
			return item.Username
		}
		return "Login"
	case storage.VaultKindCard:
		if item.CardBrand != "" && item.CardLast4 != "" {
			return item.CardBrand + " ••••" + item.CardLast4
		}
		return "Payment card"
	default:
		if item.Email != "" {
			return item.Email
		}
		return "Identity"
	}
}

func normalizeExpYear(year int) int {
	if year >= 0 && year < 100 {
		return 2000 + year
	}
	return year
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func newVaultID() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return "vlt_" + hex.EncodeToString(buf[:])
}
