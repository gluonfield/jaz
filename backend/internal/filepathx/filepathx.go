package filepathx

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func FromUserInput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		return fileURLToPath(raw, runtime.GOOS)
	}
	return raw, nil
}

func FileURI(nativePath string) string {
	return fileURI(nativePath, runtime.GOOS)
}

func fileURLToPath(raw, goos string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(parsed.Scheme, "file") {
		return "", fmt.Errorf("unsupported path URI scheme: %s", parsed.Scheme)
	}
	if goos == "windows" {
		return windowsFileURLToPath(parsed)
	}
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		path = "//" + parsed.Host + path
		path = strings.TrimSpace(path)
		if path == "" {
			return "", fmt.Errorf("path is required")
		}
		return path, nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return filepath.Clean(path), nil
}

func windowsFileURLToPath(parsed *url.URL) (string, error) {
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		path = strings.TrimPrefix(path, "/")
		if path == "" {
			return `\\` + parsed.Host, nil
		}
		return `\\` + parsed.Host + `\` + strings.ReplaceAll(path, "/", `\`), nil
	}
	if len(path) >= 3 && path[0] == '/' && isDriveLetter(path[1]) && path[2] == ':' {
		path = path[1:]
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	return strings.ReplaceAll(path, "/", `\`), nil
}

func fileURI(nativePath, goos string) string {
	if goos == "windows" {
		path := strings.ReplaceAll(nativePath, `\`, "/")
		if strings.HasPrefix(path, "//") {
			rest := strings.TrimPrefix(path, "//")
			host, share, ok := strings.Cut(rest, "/")
			if ok {
				return (&url.URL{Scheme: "file", Host: host, Path: "/" + share}).String()
			}
			return (&url.URL{Scheme: "file", Host: rest}).String()
		}
		if len(path) >= 2 && isDriveLetter(path[0]) && path[1] == ':' {
			path = "/" + path
		}
		return (&url.URL{Scheme: "file", Path: path}).String()
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(nativePath)}).String()
}

func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
