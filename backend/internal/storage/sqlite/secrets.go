package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	oauthdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationoauth"
)

const secretPrefix = "encrypted:v1:"

func (s *Store) protectSecrets() error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := oauthdb.New(s.db).WithTx(tx)
	rows, err := q.ListTokens(ctx)
	if err != nil {
		return err
	}
	keyPath := strings.TrimSpace(os.Getenv("JAZ_SECRET_KEY_FILE"))
	if keyPath == "" {
		keyPath = filepath.Join(s.root, "secrets.key")
	}
	key, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		err = nil
		for _, row := range rows {
			if strings.HasPrefix(row.TokenJson, secretPrefix) {
				return fmt.Errorf("OAuth secrets require their original key file: %s", keyPath)
			}
		}
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(keyPath), ".secrets-key-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		_, err = file.Write(key)
		if err == nil {
			err = file.Sync()
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err := os.Link(file.Name(), keyPath); err != nil {
			if !os.IsExist(err) {
				return err
			}
			key, err = os.ReadFile(keyPath)
			if err != nil {
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return err
	}
	if len(key) != 32 {
		return fmt.Errorf("OAuth encryption key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	s.secrets, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}
	changed := false
	for _, row := range rows {
		if strings.HasPrefix(row.TokenJson, secretPrefix) {
			if _, err := s.decryptSecret(row.ConnectionID, row.TokenJson); err != nil {
				return fmt.Errorf("unlock OAuth credentials: %w", err)
			}
			continue
		}
		sealed, err := s.encryptSecret(row.ConnectionID, []byte(row.TokenJson))
		if err != nil {
			return err
		}
		if err := q.SaveToken(ctx, oauthdb.SaveTokenParams{ConnectionID: row.ConnectionID, TokenJson: sealed, UpdatedAtMs: row.UpdatedAtMs}); err != nil {
			return err
		}
		changed = true
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if changed {
		if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) encryptSecret(id string, value []byte) (string, error) {
	nonce := make([]byte, s.secrets.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.secrets.Seal(nonce, nonce, value, []byte(id))
	return secretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decryptSecret(id, value string) ([]byte, error) {
	if !strings.HasPrefix(value, secretPrefix) {
		return nil, fmt.Errorf("unencrypted OAuth credential")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, secretPrefix))
	if err != nil || len(sealed) < s.secrets.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted OAuth credential")
	}
	return s.secrets.Open(nil, sealed[:s.secrets.NonceSize()], sealed[s.secrets.NonceSize():], []byte(id))
}
