package blobstore

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

var (
	ErrNotFound           = errors.New("blob not found")
	ErrInvalidKey         = errors.New("invalid blob key")
	ErrInvalidTTL         = errors.New("presign ttl must be positive")
	ErrPresignUnsupported = errors.New("blob presigning unsupported")
)

type ObjectMeta struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitzero"`
}

type PutOptions struct {
	ContentType string
}

type PresignedURL struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type Store interface {
	Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (ObjectMeta, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]ObjectMeta, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (PresignedURL, error)
	PresignPut(ctx context.Context, key string, opts PutOptions, ttl time.Duration) (PresignedURL, error)
}

func ValidateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return ErrInvalidKey
	}
	if strings.ContainsAny(key, "\\\x00") {
		return ErrInvalidKey
	}
	for segment := range strings.SplitSeq(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidKey
		}
	}
	return nil
}

func ValidatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "\\\x00") {
		return ErrInvalidKey
	}
	trimmed := strings.TrimSuffix(prefix, "/")
	if trimmed == "" {
		return ErrInvalidKey
	}
	for segment := range strings.SplitSeq(trimmed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidKey
		}
	}
	return nil
}
