package plugins

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxIconBytes = 512 << 10

func readIcon(dir, iconPath string) (string, error) {
	iconPath = strings.TrimSpace(iconPath)
	if iconPath == "" {
		return "", nil
	}
	if strings.HasPrefix(iconPath, "https://") {
		parsed, err := url.Parse(iconPath)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("%w: invalid icon URL", ErrInvalid)
		}
		return iconPath, nil
	}
	relative := strings.TrimPrefix(iconPath, "./")
	if !fs.ValidPath(relative) || strings.Contains(relative, `\`) {
		return "", fmt.Errorf("%w: invalid icon path %q", ErrInvalid, iconPath)
	}
	mimeType := iconMIME(relative)
	if mimeType == "" {
		return "", fmt.Errorf("%w: icon must be SVG, PNG or WebP", ErrInvalid)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	data, err := root.ReadFile(relative)
	if err != nil {
		return "", fmt.Errorf("%w: icon: %v", ErrInvalid, err)
	}
	if len(data) > maxIconBytes {
		return "", fmt.Errorf("%w: icon exceeds %d bytes", ErrInvalid, maxIconBytes)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func iconMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}
