package loops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MemoryPaths struct {
	dir string
}

func NewMemoryPaths(automationsDir string) *MemoryPaths {
	dir := strings.TrimSpace(automationsDir)
	if dir == "" {
		return &MemoryPaths{}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &MemoryPaths{dir: dir}
}

func (m *MemoryPaths) Dir() string {
	if m == nil {
		return ""
	}
	return m.dir
}

func (m *MemoryPaths) EnsureDir() error {
	if m == nil || m.dir == "" {
		return nil
	}
	return os.MkdirAll(m.dir, 0o755)
}

func (m *MemoryPaths) AssignMissing(loop *Loop, used map[string]struct{}) bool {
	if m == nil || m.dir == "" || loop == nil || strings.TrimSpace(loop.MemoryPath) != "" {
		return false
	}
	loop.MemoryPath = m.defaultPath(*loop, used)
	return loop.MemoryPath != ""
}

func (m *MemoryPaths) defaultPath(loop Loop, used map[string]struct{}) string {
	if m == nil || m.dir == "" {
		return ""
	}
	base := loopNameSlug(loop.Name)
	if base == "" {
		base = "loop"
	}
	candidate := filepath.Join(m.dir, base, "memory.md")
	if _, ok := used[candidate]; !ok {
		return candidate
	}
	suffix := shortLoopIDSuffix(loop.ID)
	if suffix == "" {
		suffix = "loop"
	}
	candidate = filepath.Join(m.dir, base+"-"+suffix, "memory.md")
	if _, ok := used[candidate]; !ok {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = filepath.Join(m.dir, fmt.Sprintf("%s-%s-%d", base, suffix, i), "memory.md")
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

func memoryPathSet(items []Loop) map[string]struct{} {
	used := make(map[string]struct{}, len(items))
	for _, loop := range items {
		if path := strings.TrimSpace(loop.MemoryPath); path != "" {
			used[path] = struct{}{}
		}
	}
	return used
}

func loopNameSlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	hyphen := false
	for _, r := range name {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if b.Len() > 0 && !hyphen {
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortLoopIDSuffix(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	value := b.String()
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}
