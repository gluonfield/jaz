package settings

import (
	"fmt"
	"strings"
	"unicode"
)

func ParseCommandLine(line string) (string, []string, error) {
	parts, err := commandLineFields(line)
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return parts[0], parts[1:], nil
}

func commandLineFields(line string) ([]string, error) {
	var parts []string
	var b strings.Builder
	var quote rune
	escaped := false
	inToken := false

	flush := func() {
		if inToken {
			parts = append(parts, b.String())
			b.Reset()
			inToken = false
		}
	}

	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			inToken = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			inToken = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			inToken = true
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
			inToken = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("unfinished escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %q quote", quote)
	}
	flush()
	return parts, nil
}
