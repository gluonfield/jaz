package manifest

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func Resolve(root, path string) (string, error) {
	relative := strings.TrimSuffix(strings.TrimPrefix(path, "./"), "/")
	if path == "." || path == "./" {
		relative = "."
	} else if relative == "" || strings.HasPrefix(path, "/") || !fs.ValidPath(relative) || strings.Contains(relative, `\`) {
		return "", fmt.Errorf("invalid plugin-relative path %q", path)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return "", fmt.Errorf("plugin path escapes its root: %q", path)
	}
	return absolute, nil
}
