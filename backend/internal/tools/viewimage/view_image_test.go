package viewimage

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewImageSnapshotsBlobAndKeepsBase64OutOfContent(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "bad name.png")
	data := testPNG(t, 16, 8)
	if err := os.WriteFile(imagePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (&Tool{Workspace: workspace}).Execute(context.Background(), map[string]any{
		"path":   "bad name.png",
		"detail": "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "data:image") {
		t.Fatalf("durable content contains base64 data URL: %s", result.Content)
	}
	if strings.Contains(result.Content, "media_refs") {
		t.Fatalf("visible content contains replay media refs: %s", result.Content)
	}

	var payload struct{}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if len(result.MediaRefs) != 1 {
		t.Fatalf("media refs = %#v, want one", result.MediaRefs)
	}
	ref := result.MediaRefs[0]
	if ref.BlobPath == "" || !strings.HasPrefix(ref.BlobPath, filepath.Join(workspace, ".jaz-media", "blobs")) {
		t.Fatalf("blob path %q outside workspace media dir", ref.BlobPath)
	}
	stored, err := os.ReadFile(ref.BlobPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(data) {
		t.Fatalf("stored blob = %q, want %q", stored, data)
	}
	if ref.Detail != "high" || ref.MimeType != "image/png" || ref.OriginalPath != "bad name.png" {
		t.Fatalf("unexpected ref metadata: %#v", ref)
	}
	if ref.Width != 16 || ref.Height != 8 {
		t.Fatalf("prompt dimensions = %dx%d, want 16x8", ref.Width, ref.Height)
	}
}

func TestViewImageDownscalesLargeImageBeforePersistingMediaRef(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "large.png")
	data := testPNG(t, 4096, 1024)
	if err := os.WriteFile(imagePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (&Tool{Workspace: workspace}).Execute(context.Background(), map[string]any{
		"path": "large.png",
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		MimeType         string `json:"mime_type"`
		Size             int    `json:"size"`
		PromptMimeType   string `json:"prompt_mime_type"`
		PromptSize       int    `json:"prompt_size"`
		PromptWidth      int    `json:"prompt_width"`
		PromptHeight     int    `json:"prompt_height"`
		PromptWasResized bool   `json:"prompt_was_resized"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MimeType != "image/png" || payload.Size != len(data) {
		t.Fatalf("source metadata = %#v, want source png and original size", payload)
	}
	if !payload.PromptWasResized || payload.PromptMimeType != "image/png" || payload.PromptWidth != 2048 || payload.PromptHeight != 512 {
		t.Fatalf("prompt metadata = %#v, want resized 2048x512 png", payload)
	}
	if len(result.MediaRefs) != 1 {
		t.Fatalf("media refs = %#v, want one", result.MediaRefs)
	}
	ref := result.MediaRefs[0]
	if ref.Size != int64(payload.PromptSize) || ref.Width != 2048 || ref.Height != 512 {
		t.Fatalf("media ref = %#v, want prompt ref metadata", ref)
	}
	stored, err := os.ReadFile(ref.BlobPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 2048 || cfg.Height != 512 {
		t.Fatalf("stored prompt image dimensions = %dx%d, want 2048x512", cfg.Width, cfg.Height)
	}
}

func TestViewImageRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, testPNG(t, 4, 4), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := (&Tool{Workspace: workspace}).Execute(context.Background(), map[string]any{"path": outside})
	if err == nil {
		t.Fatal("expected workspace escape to be rejected")
	}
}

func TestViewImageRejectsUnsupportedType(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := (&Tool{Workspace: workspace}).Execute(context.Background(), map[string]any{"path": "note.txt"})
	if err == nil || !strings.Contains(err.Error(), "unsupported image type") {
		t.Fatalf("err = %v, want unsupported image type", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
