package acpadapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (m *Manager) installArchive(ctx context.Context, spec adapterSpec) error {
	body, err := m.download(ctx, spec)
	if err != nil {
		return err
	}
	if err := verifySHA256(body, spec.SHA256); err != nil {
		return fmt.Errorf("verify %s adapter archive: %w", spec.Adapter, err)
	}
	parent := filepath.Dir(spec.Root)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := extractArchive(body, tmp); err != nil {
		return err
	}
	if err := chmodLaunchFiles(tmp, spec); err != nil {
		return err
	}
	_ = os.RemoveAll(spec.Root)
	return os.Rename(tmp, spec.Root)
}

func (m *Manager) download(ctx context.Context, spec adapterSpec) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return nil, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download adapter: %s", res.Status)
	}
	total := res.ContentLength
	if total < 0 {
		total = 0
	}
	m.setDownloadProgress(spec, 0, total)
	var body bytes.Buffer
	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
			downloaded += int64(n)
			m.setDownloadProgress(spec, downloaded, total)
		}
		if readErr == io.EOF {
			return body.Bytes(), nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func verifySHA256(body []byte, wantHex string) error {
	want, err := hex.DecodeString(strings.TrimSpace(wantHex))
	if err != nil {
		return err
	}
	got := sha256.Sum256(body)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("sha256 mismatch")
	}
	return nil
}

func extractArchive(body []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if !cleanRelative(name) {
			return fmt.Errorf("adapter archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func chmodLaunchFiles(tmpRoot string, spec adapterSpec) error {
	paths := []string{spec.Command}
	for _, value := range spec.Env {
		paths = append(paths, value)
	}
	for _, value := range paths {
		rel, err := filepath.Rel(spec.Root, value)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("adapter launch path escapes install root")
		}
		if err := os.Chmod(filepath.Join(tmpRoot, rel), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
