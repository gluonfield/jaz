package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/pathsafe"
)

type LocalFSStore struct {
	objectsDir string
	metaDir    string
	mu         sync.RWMutex
}

var _ Store = (*LocalFSStore)(nil)

func NewLocalFSStore(root string) (*LocalFSStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("blobstore root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &LocalFSStore{
		objectsDir: filepath.Join(abs, "objects"),
		metaDir:    filepath.Join(abs, "meta"),
	}, nil
}

func (s *LocalFSStore) Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return ObjectMeta{}, err
	}
	if body == nil {
		return ObjectMeta{}, fmt.Errorf("blob body is required")
	}
	objPath, err := s.objectPath(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	metaPath, err := s.metaPath(key)
	if err != nil {
		return ObjectMeta{}, err
	}

	hasher := sha256.New()
	var written int64
	tempObj, err := stageTempFile(filepath.Dir(objPath), ".blob-*", func(w io.Writer) error {
		n, copyErr := io.Copy(io.MultiWriter(w, hasher), ctxReader{ctx: ctx, r: body})
		if copyErr != nil {
			return copyErr
		}
		written = n
		return ctx.Err()
	})
	if err != nil {
		return ObjectMeta{}, err
	}
	keepObj := false
	defer func() {
		if !keepObj {
			_ = os.Remove(tempObj)
		}
	}()

	contentType := strings.TrimSpace(opts.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(key))
	}
	digest := hex.EncodeToString(hasher.Sum(nil))

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ObjectMeta{}, err
	}
	if err := os.Rename(tempObj, objPath); err != nil {
		return ObjectMeta{}, err
	}
	keepObj = true

	info, err := os.Stat(objPath)
	if err != nil {
		return ObjectMeta{}, err
	}
	meta := ObjectMeta{
		Key:         key,
		Size:        written,
		ContentType: contentType,
		SHA256:      digest,
		UpdatedAt:   info.ModTime().UTC(),
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return ObjectMeta{}, err
	}
	tempMeta, err := stageTempFile(filepath.Dir(metaPath), ".meta-*", func(w io.Writer) error {
		_, writeErr := w.Write(encoded)
		return writeErr
	})
	if err != nil {
		return ObjectMeta{}, err
	}
	if err := os.Rename(tempMeta, metaPath); err != nil {
		_ = os.Remove(tempMeta)
		return ObjectMeta{}, err
	}
	return meta, nil
}

func (s *LocalFSStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, ObjectMeta{}, err
	}
	if err := ValidateKey(key); err != nil {
		return nil, ObjectMeta{}, err
	}
	objPath, err := s.objectPath(key)
	if err != nil {
		return nil, ObjectMeta{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	file, err := os.Open(objPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ObjectMeta{}, ErrNotFound
		}
		return nil, ObjectMeta{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, ObjectMeta{}, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, ObjectMeta{}, ErrNotFound
	}
	meta := s.readMeta(key, info)
	return file, meta, nil
}

func (s *LocalFSStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateKey(key); err != nil {
		return err
	}
	objPath, err := s.objectPath(key)
	if err != nil {
		return err
	}
	metaPath, err := s.metaPath(key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(objPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Remove(metaPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalFSStore) List(ctx context.Context, prefix string) ([]ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidatePrefix(prefix); err != nil {
		return nil, err
	}
	walkRoot := s.objectsDir
	if idx := strings.LastIndexByte(prefix, '/'); idx >= 0 {
		candidate := filepath.Join(s.objectsDir, filepath.FromSlash(prefix[:idx]))
		if !pathsafe.Within(s.objectsDir, candidate) {
			return nil, ErrInvalidKey
		}
		walkRoot = candidate
	}
	if _, err := os.Stat(walkRoot); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []ObjectMeta
	err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if path != s.objectsDir && prefix != "" {
				relDir, relErr := filepath.Rel(s.objectsDir, path)
				if relErr == nil {
					dirPrefix := filepath.ToSlash(relDir) + "/"
					if !strings.HasPrefix(dirPrefix, prefix) && !strings.HasPrefix(prefix, dirPrefix) {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".blob-") {
			return nil
		}
		rel, err := filepath.Rel(s.objectsDir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		s.mu.RLock()
		info, infoErr := entry.Info()
		var meta ObjectMeta
		if infoErr == nil {
			meta = s.readMeta(key, info)
		}
		s.mu.RUnlock()
		if infoErr != nil {
			if errors.Is(infoErr, fs.ErrNotExist) {
				return nil
			}
			return infoErr
		}
		out = append(out, meta)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b ObjectMeta) int {
		return strings.Compare(a.Key, b.Key)
	})
	return out, nil
}

func (s *LocalFSStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return PresignedURL{}, err
	}
	if err := ValidateKey(key); err != nil {
		return PresignedURL{}, err
	}
	if ttl <= 0 {
		return PresignedURL{}, ErrInvalidTTL
	}
	return PresignedURL{}, ErrPresignUnsupported
}

func (s *LocalFSStore) PresignPut(ctx context.Context, key string, _ PutOptions, ttl time.Duration) (PresignedURL, error) {
	if err := ctx.Err(); err != nil {
		return PresignedURL{}, err
	}
	if err := ValidateKey(key); err != nil {
		return PresignedURL{}, err
	}
	if ttl <= 0 {
		return PresignedURL{}, ErrInvalidTTL
	}
	return PresignedURL{}, ErrPresignUnsupported
}

func (s *LocalFSStore) objectPath(key string) (string, error) {
	target := filepath.Join(s.objectsDir, filepath.FromSlash(key))
	if !pathsafe.Within(s.objectsDir, target) {
		return "", ErrInvalidKey
	}
	return target, nil
}

func (s *LocalFSStore) metaPath(key string) (string, error) {
	target := filepath.Join(s.metaDir, filepath.FromSlash(key)+".json")
	if !pathsafe.Within(s.metaDir, target) {
		return "", ErrInvalidKey
	}
	return target, nil
}

func (s *LocalFSStore) readMeta(key string, info fs.FileInfo) ObjectMeta {
	fallback := ObjectMeta{
		Key:         key,
		Size:        info.Size(),
		ContentType: mime.TypeByExtension(filepath.Ext(key)),
		UpdatedAt:   info.ModTime().UTC(),
	}
	metaPath, err := s.metaPath(key)
	if err != nil {
		return fallback
	}
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return fallback
	}
	var meta ObjectMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fallback
	}
	meta.Key = key
	meta.Size = info.Size()
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = info.ModTime().UTC()
	}
	return meta
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func stageTempFile(dir, pattern string, write func(io.Writer) error) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := write(temp); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	keep = true
	return tempPath, nil
}
