package integrations

import "strings"

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
