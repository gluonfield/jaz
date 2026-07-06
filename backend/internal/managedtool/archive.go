package managedtool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) install(ctx context.Context, spec toolSpec) error {
	body, err := m.download(ctx, spec.URL)
	if err != nil {
		return err
	}
	if err := verifySHA512(body, spec.SHA512); err != nil {
		return fmt.Errorf("verify %s: %w", DisplayName(spec.Tool), err)
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
	if err := installPayload(body, spec, filepath.Join(tmp, ExecutableName(spec.Tool))); err != nil {
		return err
	}
	_ = os.RemoveAll(spec.Root)
	return os.Rename(tmp, spec.Root)
}

func (m *Manager) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, res.Status)
	}
	return io.ReadAll(res.Body)
}

func verifySHA512(body []byte, wantHex string) error {
	want, err := hex.DecodeString(strings.TrimSpace(wantHex))
	if err != nil {
		return err
	}
	got := sha512.Sum512(body)
	if !bytes.Equal(got[:], want) {
		return fmt.Errorf("sha512 mismatch")
	}
	return nil
}

func installPayload(body []byte, spec toolSpec, dst string) error {
	if strings.Contains(strings.ToLower(spec.URL), ".tar.gz") {
		return extractAntigravityTarball(body, dst)
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil {
		return err
	}
	return nil
}

func extractAntigravityTarball(body []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("Antigravity CLI archive did not contain antigravity")
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if name == "." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return fmt.Errorf("Antigravity CLI archive contains unsafe path %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg || path.Base(name) != "antigravity" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	}
}

func downloadingStatus(spec toolSpec) Status {
	return Status{
		Tool:      spec.Tool,
		Version:   spec.Version,
		Platform:  spec.Platform,
		Path:      spec.Command,
		State:     StateDownloading,
		Message:   "Downloading " + DisplayName(spec.Tool),
		StartedAt: time.Now().UTC(),
	}
}

func readyStatus(spec toolSpec) Status {
	return Status{
		Tool:       spec.Tool,
		Version:    spec.Version,
		Platform:   spec.Platform,
		Path:       spec.Command,
		State:      StateReady,
		Message:    DisplayName(spec.Tool) + " is ready",
		FinishedAt: time.Now().UTC(),
	}
}

func failedStatus(spec toolSpec, err error) Status {
	return Status{
		Tool:       spec.Tool,
		Version:    spec.Version,
		Platform:   spec.Platform,
		Path:       spec.Command,
		State:      StateFailed,
		Message:    err.Error(),
		FinishedAt: time.Now().UTC(),
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
