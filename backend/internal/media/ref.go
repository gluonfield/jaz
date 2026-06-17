package media

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const (
	TypeInputImage = "input_image"
)

type Ref struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	BlobPath     string `json:"blob_path"`
	OriginalPath string `json:"path,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SHA256       string `json:"sha256"`
	Detail       string `json:"detail,omitempty"`
	Filename     string `json:"filename,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

type Materialized struct {
	ImageURL string
	Detail   string
	Filename string
}

func CloneRefs(refs []Ref) []Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make([]Ref, len(refs))
	copy(out, refs)
	return out
}

func CloneRefMap(refs map[string][]Ref) map[string][]Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make(map[string][]Ref, len(refs))
	for id, values := range refs {
		out[id] = CloneRefs(values)
	}
	return out
}

func MaterializeRef(ref Ref) (Materialized, error) {
	if ref.Type != TypeInputImage {
		return Materialized{}, fmt.Errorf("unsupported media ref type %q", ref.Type)
	}
	if strings.TrimSpace(ref.BlobPath) == "" {
		return Materialized{}, fmt.Errorf("media ref missing blob_path")
	}
	if strings.TrimSpace(ref.SHA256) == "" {
		return Materialized{}, fmt.Errorf("media ref missing sha256")
	}

	detail, err := replayImageDetail(ref.Detail)
	if err != nil {
		return Materialized{}, err
	}

	info, err := os.Lstat(ref.BlobPath)
	if err != nil {
		return Materialized{}, fmt.Errorf("media blob is not readable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Materialized{}, fmt.Errorf("media blob is not a regular file")
	}
	if ref.Size > 0 && info.Size() != ref.Size {
		return Materialized{}, fmt.Errorf("media blob size changed: got %d, want %d", info.Size(), ref.Size)
	}

	data, err := os.ReadFile(ref.BlobPath)
	if err != nil {
		return Materialized{}, fmt.Errorf("media blob is not readable: %w", err)
	}
	sum := sha256.Sum256(data)
	if got, want := hex.EncodeToString(sum[:]), strings.ToLower(ref.SHA256); got != want {
		return Materialized{}, fmt.Errorf("media blob hash changed: got %s, want %s", got, want)
	}

	mimeType := strings.TrimSpace(ref.MimeType)
	if mimeType == "" {
		return Materialized{}, fmt.Errorf("media ref missing mime_type")
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return Materialized{}, fmt.Errorf("media ref mime_type %q is not an image", mimeType)
	}

	return Materialized{
		ImageURL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
		Detail:   detail,
		Filename: ref.Filename,
	}, nil
}

func replayImageDetail(detail string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(detail)) {
	case "", "auto":
		return "auto", nil
	case "low":
		return "low", nil
	case "high":
		return "high", nil
	case "original":
		return "", fmt.Errorf("image detail %q is not supported by the chat-completions image path", detail)
	default:
		return "", fmt.Errorf("unknown image detail %q", detail)
	}
}
