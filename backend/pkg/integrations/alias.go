package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func NormalizeAlias(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	dash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			out.WriteRune(r)
			dash = false
			continue
		}
		if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func DefaultAlias(accountName, accountID string) string {
	if before, _, ok := strings.Cut(strings.TrimSpace(accountName), "@"); ok {
		if alias := NormalizeAlias(before); alias != "" {
			return alias
		}
	}
	if alias := NormalizeAlias(accountName); alias != "" {
		return alias
	}
	return NormalizeAlias(accountID)
}

func ConnectionID(provider, accountID string) (string, error) {
	accountID = strings.ToLower(strings.TrimSpace(accountID))
	alias := NormalizeAlias(accountID)
	if alias == "" {
		return "", fmt.Errorf("%s account id is required", provider)
	}
	sum := sha256.Sum256([]byte(accountID))
	return fmt.Sprintf("%s:%s-%x", provider, alias, sum[:4]), nil
}

func SourceSlug(value string) string {
	value = strings.TrimSpace(value)
	slug := NormalizeAlias(value)
	if slug == "" {
		slug = "source"
	}
	if len(slug) > 72 {
		slug = strings.Trim(slug[:72], "-")
	}
	sum := sha256.Sum256([]byte(value))
	return slug + "-" + hex.EncodeToString(sum[:4])
}
