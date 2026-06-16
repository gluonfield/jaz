package runtimeauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	APIKey string `json:"api_key"`
}

func Path(root string) string {
	return filepath.Join(strings.TrimSpace(root), "auth.json")
}

func Ensure(root string) (string, error) {
	path := Path(root)
	if data, err := os.ReadFile(path); err == nil {
		var file File
		if err := json.Unmarshal(data, &file); err != nil {
			return "", err
		}
		if key := strings.TrimSpace(file.APIKey); key != "" {
			return key, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	key, err := generateKey()
	if err != nil {
		return "", err
	}
	return key, Save(path, key)
}

func Save(path, key string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(File{APIKey: strings.TrimSpace(key)}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "auth-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
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

func generateKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
