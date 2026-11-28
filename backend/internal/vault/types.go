package vault

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var (
	ErrInvalidInput    = errors.New("invalid vault item input")
	ErrOriginMismatch  = errors.New("vault item origin does not match active browser page")
	ErrApprovalDenied  = errors.New("vault autofill was denied by user")
	ErrSessionRequired = errors.New("vault action requires an active Jaz thread")
)

var nonDigitRe = regexp.MustCompile(`\D+`)

type CreateInput struct {
	Kind           string   `json:"kind"`
	Label          string   `json:"label"`
	Origins        []string `json:"origins,omitempty"`
	Username       string   `json:"username,omitempty"`
	Password       string   `json:"password,omitempty"`
	TOTPSecret     string   `json:"totp_secret,omitempty"`
	CardholderName string   `json:"cardholder_name,omitempty"`
	PAN            string   `json:"pan,omitempty"`
	CardExpMonth   int      `json:"card_exp_month,omitempty"`
	CardExpYear    int      `json:"card_exp_year,omitempty"`
	CVV            string   `json:"cvv,omitempty"`
	BillingAddress string   `json:"billing_address,omitempty"`
	BillingSummary string   `json:"billing_summary,omitempty"`
	Email          string   `json:"email,omitempty"`
	FullName       string   `json:"full_name,omitempty"`
	Phone          string   `json:"phone,omitempty"`
	AddressLines   string   `json:"address_lines,omitempty"`
	PostalCode     string   `json:"postal_code,omitempty"`
	Country        string   `json:"country,omitempty"`
	CityCountry    string   `json:"city_country,omitempty"`
}

type UpdateInput struct {
	Label          *string  `json:"label,omitempty"`
	Origins        []string `json:"origins,omitempty"`
	ReplaceOrigins bool     `json:"replace_origins,omitempty"`
	Username       *string  `json:"username,omitempty"`
	Password       *string  `json:"password,omitempty"`
	TOTPSecret     *string  `json:"totp_secret,omitempty"`
	CardholderName *string  `json:"cardholder_name,omitempty"`
	PAN            *string  `json:"pan,omitempty"`
	CardExpMonth   *int     `json:"card_exp_month,omitempty"`
	CardExpYear    *int     `json:"card_exp_year,omitempty"`
	CVV            *string  `json:"cvv,omitempty"`
	BillingAddress *string  `json:"billing_address,omitempty"`
	BillingSummary *string  `json:"billing_summary,omitempty"`
	Email          *string  `json:"email,omitempty"`
	FullName       *string  `json:"full_name,omitempty"`
	Phone          *string  `json:"phone,omitempty"`
	AddressLines   *string  `json:"address_lines,omitempty"`
	PostalCode     *string  `json:"postal_code,omitempty"`
	Country        *string  `json:"country,omitempty"`
	CityCountry    *string  `json:"city_country,omitempty"`
}

func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: origin is required", ErrInvalidInput)
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: invalid origin %q", ErrInvalidInput, raw)
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	switch scheme {
	case "https":
	case "http":
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return "", fmt.Errorf("%w: origin %q must use HTTPS or localhost", ErrInvalidInput, raw)
		}
	default:
		return "", fmt.Errorf("%w: unsupported origin scheme %q", ErrInvalidInput, u.Scheme)
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return scheme + "://" + host + ":" + port, nil
	}
	return scheme + "://" + host, nil
}

func NormalizeOrigins(origins []string) ([]string, error) {
	seen := make(map[string]bool, len(origins))
	out := make([]string, 0, len(origins))
	for _, raw := range origins {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		norm, err := NormalizeOrigin(raw)
		if err != nil {
			return nil, err
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out, nil
}

// OriginMatchesLogin enforces exact matching origin (scheme + host + port) for silent login autofill.
func OriginMatchesLogin(itemOrigins []string, pageOrigin string) bool {
	normPage, err := NormalizeOrigin(pageOrigin)
	if err != nil {
		return false
	}
	for _, allowed := range itemOrigins {
		normAllowed, err := NormalizeOrigin(allowed)
		if err == nil && normAllowed == normPage {
			return true
		}
	}
	return false
}

// DomainMatchesFilter matches exact origin or exact hostname when listing items by domain.
func DomainMatchesFilter(itemOrigins []string, domain string) bool {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return true
	}
	normQuery, err := NormalizeOrigin(domain)
	if err != nil {
		return false
	}
	queryURL, err := url.Parse(normQuery)
	if err != nil {
		return false
	}
	queryHost := strings.ToLower(queryURL.Hostname())
	hasScheme := strings.Contains(domain, "://")
	for _, raw := range itemOrigins {
		normItem, err := NormalizeOrigin(raw)
		if err != nil {
			continue
		}
		if normItem == normQuery {
			return true
		}
		if !hasScheme {
			if itemURL, err := url.Parse(normItem); err == nil && strings.ToLower(itemURL.Hostname()) == queryHost {
				return true
			}
		}
	}
	return false
}

func NormalizeDigits(raw string) string {
	return nonDigitRe.ReplaceAllString(strings.TrimSpace(raw), "")
}

func DetectCardBrand(pan string) string {
	switch {
	case strings.HasPrefix(pan, "4"):
		return "Visa"
	case len(pan) >= 2 && (pan[:2] == "34" || pan[:2] == "37"):
		return "Amex"
	case len(pan) >= 2 && pan[:2] >= "51" && pan[:2] <= "55":
		return "Mastercard"
	case len(pan) >= 4 && pan[:4] >= "2221" && pan[:4] <= "2720":
		return "Mastercard"
	case strings.HasPrefix(pan, "6011") || strings.HasPrefix(pan, "65") || (len(pan) >= 3 && pan[:3] >= "644" && pan[:3] <= "649"):
		return "Discover"
	case len(pan) >= 4 && pan[:4] >= "3528" && pan[:4] <= "3589":
		return "JCB"
	case strings.HasPrefix(pan, "36") || strings.HasPrefix(pan, "38") || (len(pan) >= 3 && pan[:3] >= "300" && pan[:3] <= "305"):
		return "Diners"
	default:
		return "Card"
	}
}

func SummarizeBillingAddress(address, explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	lines := strings.Split(strings.ReplaceAll(address, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			if len(line) > 80 {
				return line[:80]
			}
			return line
		}
	}
	return ""
}

func NormalizeKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case storage.VaultKindLogin:
		return storage.VaultKindLogin, nil
	case storage.VaultKindCard:
		return storage.VaultKindCard, nil
	case storage.VaultKindIdentity:
		return storage.VaultKindIdentity, nil
	default:
		return "", fmt.Errorf("%w: kind must be login, card, or identity", ErrInvalidInput)
	}
}

// GenerateTOTP computes a standard 6-digit RFC 6238 SHA-1 TOTP code (30s period).
func GenerateTOTP(secret string, at time.Time) (string, error) {
	cleaned := strings.TrimSpace(secret)
	if strings.HasPrefix(strings.ToLower(cleaned), "otpauth://") {
		if u, err := url.Parse(cleaned); err == nil {
			if param := strings.TrimSpace(u.Query().Get("secret")); param != "" {
				cleaned = param
			}
		}
	}
	cleaned = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(cleaned))
	if cleaned == "" {
		return "", errors.New("TOTP secret is empty")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(cleaned, "="))
	if err != nil || len(key) == 0 {
		return "", errors.New("invalid base32 TOTP secret")
	}
	defer clear(key)
	counter := uint64(at.Unix() / 30)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset]&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])) % 1_000_000
	return fmt.Sprintf("%06d", code), nil
}
