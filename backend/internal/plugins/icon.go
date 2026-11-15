package plugins

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func readIcon(dir, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	relative, ok := strings.CutPrefix(path, "./")
	if !ok || !fs.ValidPath(relative) || strings.Contains(relative, `\`) || !strings.HasSuffix(relative, ".svg") {
		return "", fmt.Errorf("%w: icon must be a ./ path to an SVG inside the plugin", ErrInvalid)
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
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data), nil
}
