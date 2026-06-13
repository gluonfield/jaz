package runtimeenv

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

func Path(root string) string {
	return filepath.Join(strings.TrimSpace(root), ".env")
}

func Lookup(path, key string) (string, bool) {
	env, err := godotenv.Read(path)
	if err != nil {
		return "", false
	}
	value, ok := env[key]
	return value, ok && strings.TrimSpace(value) != ""
}

func Save(path string, values map[string]string) error {
	env, err := godotenv.Read(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		env = map[string]string{}
	}
	for key, value := range values {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			env[key] = strings.TrimSpace(value)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content, err := godotenv.Marshal(env)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
