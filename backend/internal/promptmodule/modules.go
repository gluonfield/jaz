package promptmodule

import "strings"

// Modules is runtime-only prompt context appended to a base system prompt.
type Modules []string

func New(parts ...string) Modules {
	out := make(Modules, 0, len(parts))
	for _, part := range parts {
		if part := strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (m Modules) Append(parts ...string) Modules {
	out := New(m...)
	return append(out, New(parts...)...)
}

func (m Modules) Strings() []string {
	return append([]string(nil), New(m...)...)
}

func (m Modules) Text() string {
	return strings.Join(m.Strings(), "\n\n")
}
