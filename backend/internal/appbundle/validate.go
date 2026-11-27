package appbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func hasWindowsDrivePrefix(raw string) bool {
	if len(raw) < 2 || raw[1] != ':' {
		return false
	}
	c := raw[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func ValidatePath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if strings.ContainsAny(raw, "\x00\r\n\\") {
		return "", fmt.Errorf("%w: disallowed control or backslash character in %q", ErrInvalidPath, raw)
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return "", fmt.Errorf("%w: control character in %q", ErrInvalidPath, raw)
		}
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != raw {
		return "", fmt.Errorf("%w: leading or trailing whitespace in %q", ErrInvalidPath, raw)
	}
	if strings.HasPrefix(raw, "/") || strings.HasSuffix(raw, "/") || strings.Contains(raw, "//") || filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" || hasWindowsDrivePrefix(raw) {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, raw)
	}
	if !fs.ValidPath(raw) || raw == "." {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, raw)
	}
	lower := strings.ToLower(raw)
	if lower == ".jaz" || strings.HasPrefix(lower, ".jaz/") {
		return "", fmt.Errorf("%w: .jaz/ prefix is reserved for host viewer assets", ErrInvalidPath)
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: %q", ErrInvalidPath, raw)
		}
		if len(seg) > 255 {
			return "", fmt.Errorf("%w: path segment too long", ErrInvalidPath)
		}
	}
	return raw, nil
}

func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrInvalidManifest)
	}
	m.Format = strings.TrimSpace(m.Format)
	if m.Format != Format {
		return fmt.Errorf("%w: format must be %q", ErrInvalidManifest, Format)
	}
	m.Title = strings.TrimSpace(m.Title)
	if m.Title == "" || utf8.RuneCountInString(m.Title) > MaxTitleRunes {
		return fmt.Errorf("%w: title must be between 1 and %d characters", ErrInvalidManifest, MaxTitleRunes)
	}
	m.Description = strings.TrimSpace(m.Description)
	if utf8.RuneCountInString(m.Description) > MaxDescRunes {
		return fmt.Errorf("%w: description exceeds %d characters", ErrInvalidManifest, MaxDescRunes)
	}
	m.Kind = strings.ToLower(strings.TrimSpace(m.Kind))
	if m.Kind != KindArtifact && m.Kind != KindDocument {
		return fmt.Errorf("%w: kind must be %q or %q", ErrInvalidManifest, KindArtifact, KindDocument)
	}
	entry, err := ValidatePath(strings.TrimSpace(m.Entry))
	if err != nil {
		return fmt.Errorf("%w: invalid entry path: %v", ErrInvalidManifest, err)
	}
	if entry == ManifestPath {
		return fmt.Errorf("%w: entry cannot be %s", ErrInvalidManifest, ManifestPath)
	}
	ext := strings.ToLower(path.Ext(entry))
	switch m.Kind {
	case KindArtifact:
		if ext != ".html" && ext != ".htm" && ext != ".svg" {
			return fmt.Errorf("%w: artifact entry must end in .html, .htm, or .svg", ErrInvalidManifest)
		}
	case KindDocument:
		if ext != ".md" && ext != ".markdown" && ext != ".html" && ext != ".htm" {
			return fmt.Errorf("%w: document entry must end in .md, .markdown, .html, or .htm", ErrInvalidManifest)
		}
	}
	m.Entry = entry

	if len(m.Capabilities) > MaxCapabilities {
		return fmt.Errorf("%w: too many capabilities", ErrInvalidManifest)
	}
	seenCaps := make(map[string]bool, len(m.Capabilities))
	cleanCaps := make([]string, 0, len(m.Capabilities))
	for _, rawCap := range m.Capabilities {
		c := strings.ToLower(strings.TrimSpace(rawCap))
		if !validCapabilities[c] {
			return fmt.Errorf("%w: unsupported capability %q", ErrInvalidManifest, rawCap)
		}
		if !seenCaps[c] {
			seenCaps[c] = true
			cleanCaps = append(cleanCaps, c)
		}
	}
	sort.Strings(cleanCaps)
	m.Capabilities = cleanCaps

	if len(m.AllowedTools) > MaxAllowedTools {
		return fmt.Errorf("%w: too many allowed_tools (max %d)", ErrInvalidManifest, MaxAllowedTools)
	}
	seenTools := make(map[string]bool, len(m.AllowedTools))
	cleanTools := make([]string, 0, len(m.AllowedTools))
	for _, rawTool := range m.AllowedTools {
		t := strings.TrimSpace(rawTool)
		if !isValidToolName(t) {
			return fmt.Errorf("%w: invalid tool name %q in allowed_tools", ErrInvalidManifest, rawTool)
		}
		if !seenTools[t] {
			seenTools[t] = true
			cleanTools = append(cleanTools, t)
		}
	}
	sort.Strings(cleanTools)
	m.AllowedTools = cleanTools

	if m.Icon != nil {
		if err := ValidateIcon(m.Icon); err != nil {
			return err
		}
	}
	return nil
}

func ValidateIcon(ic *IconSpec) error {
	ic.Kind = strings.ToLower(strings.TrimSpace(ic.Kind))
	ic.Value = strings.TrimSpace(ic.Value)
	ic.Color = strings.TrimSpace(ic.Color)
	ic.Background = strings.TrimSpace(ic.Background)
	if ic.Kind == "" && ic.Value == "" {
		return nil
	}
	switch ic.Kind {
	case "emoji", "initials", "glyph":
		if ic.Value == "" || utf8.RuneCountInString(ic.Value) > 64 {
			return fmt.Errorf("%w: icon value must be 1..64 characters", ErrInvalidManifest)
		}
	case "path":
		cleaned, err := ValidatePath(ic.Value)
		if err != nil {
			return fmt.Errorf("%w: invalid icon path: %v", ErrInvalidManifest, err)
		}
		ext := strings.ToLower(path.Ext(cleaned))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".svg":
		default:
			return fmt.Errorf("%w: icon path must be a png, jpg, webp, or svg file", ErrInvalidManifest)
		}
		ic.Value = cleaned
	default:
		return fmt.Errorf("%w: unsupported icon kind %q", ErrInvalidManifest, ic.Kind)
	}
	if len(ic.Color) > 32 || len(ic.Background) > 32 || strings.ContainsAny(ic.Color+ic.Background, ";<>\"'\r\n") {
		return fmt.Errorf("%w: invalid icon color/background", ErrInvalidManifest)
	}
	return nil
}

func isValidToolName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	if strings.Contains(name, "*") || strings.ContainsAny(name, " \t\r\n/\\") {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		ok := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == ':'
		if !ok {
			return false
		}
	}
	return true
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func ComputeBundleSHA256(files []FileEntry) string {
	sorted := make([]FileEntry, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	h := sha256.New()
	h.Write([]byte(Format + "\n"))
	for _, f := range sorted {
		fileHash := f.SHA256
		if fileHash == "" {
			fileHash = sha256HexBytes(f.Data)
		}
		h.Write([]byte(f.Path))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatInt(f.SizeBytes, 10)))
		h.Write([]byte{0})
		h.Write([]byte(fileHash))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

var bundleExtContentTypes = map[string]string{
	".html":     "text/html; charset=utf-8",
	".htm":      "text/html; charset=utf-8",
	".css":      "text/css; charset=utf-8",
	".js":       "text/javascript; charset=utf-8",
	".mjs":      "text/javascript; charset=utf-8",
	".json":     "application/json; charset=utf-8",
	".md":       "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
	".txt":      "text/plain; charset=utf-8",
	".csv":      "text/csv; charset=utf-8",
	".svg":      "image/svg+xml",
	".png":      "image/png",
	".jpg":      "image/jpeg",
	".jpeg":     "image/jpeg",
	".webp":     "image/webp",
	".gif":      "image/gif",
	".ico":      "image/x-icon",
	".woff":     "font/woff",
	".woff2":    "font/woff2",
	".ttf":      "font/ttf",
	".otf":      "font/otf",
	".wasm":     "application/wasm",
	".glb":      "model/gltf-binary",
	".gltf":     "model/gltf+json",
}

func ResolveContentType(relpath string) string {
	ext := strings.ToLower(path.Ext(relpath))
	if ct, ok := bundleExtContentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func canonicalManifestBytes(m Manifest) (json.RawMessage, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}
