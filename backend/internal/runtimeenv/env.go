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
	return write(path, env)
}

// Remove deletes the given keys from the env file. A no-op when the file or
// keys are absent; if the file ends up empty it is removed entirely.
func Remove(path string, keys ...string) error {
	env, err := godotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	changed := false
	for _, key := range keys {
		if _, ok := env[key]; ok {
			delete(env, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(env) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return write(path, env)
}

func write(path string, env map[string]string) error {
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
