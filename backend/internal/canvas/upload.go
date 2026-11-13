package canvas

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

const uploadsDir = "uploads"

// SaveUpload keeps a file the user put on the board inside the canvas folder,
// where the agent can read it, and returns its path relative to that folder.
// A name already taken gets a number.
func (s *Service) SaveUpload(sessionID, name string, content io.Reader) (string, error) {
	dir := filepath.Join(s.Dir(sessionID), uploadsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name = uploadName(name)
	ext := filepath.Ext(name)
	for n := 1; ; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
		}
		target := filepath.Join(dir, candidate)
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = io.Copy(file, content)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(target)
			return "", err
		}
		return path.Join(uploadsDir, candidate), nil
	}
}

func uploadName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, ".-")
	if name == "" {
		return "file"
	}
	return name
}
