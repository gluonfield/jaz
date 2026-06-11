package viewimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/tools"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	maxImageBytes        int64 = 10 << 20
	maxPromptImageWidth        = 2048
	maxPromptImageHeight       = 2048
)

type Tool struct {
	Workspace string
}

func (t *Tool) Definition() tools.Definition {
	return tools.Function(
		"view_image",
		"Loads an image from the server-side session working directory so the model can inspect it visually.",
		false,
		tools.ObjectSchema(map[string]any{
			"path": tools.StringSchema("Path to a server-side image file inside the session working directory."),
			"detail": map[string]any{
				"type":        "string",
				"description": "Image detail budget: auto, low, or high. Defaults to high.",
				"enum":        []string{"auto", "low", "high"},
			},
		}, []string{"path"}),
	)
}

func (t *Tool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	select {
	case <-ctx.Done():
		return tools.Result{}, ctx.Err()
	default:
	}

	workspace := strings.TrimSpace(t.Workspace)
	if workspace == "" {
		return tools.Result{}, errors.New("workspace is not configured")
	}
	inputPath := strings.TrimSpace(tools.StringInput(inputs, "path"))
	if inputPath == "" {
		return tools.Result{}, errors.New("path is required")
	}
	detail, err := imageDetail(tools.StringInput(inputs, "detail"))
	if err != nil {
		return tools.Result{}, err
	}

	base, err := sessioncontext.SessionBase(ctx, workspace)
	if err != nil {
		return tools.Result{}, err
	}
	path, err := pathsafe.Resolve(base, inputPath)
	if err != nil {
		return tools.Result{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return tools.Result{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return tools.Result{}, fmt.Errorf("image path %q is a symlink", inputPath)
	}
	if !info.Mode().IsRegular() {
		return tools.Result{}, fmt.Errorf("image path %q is not a regular file", inputPath)
	}
	if info.Size() <= 0 {
		return tools.Result{}, fmt.Errorf("image path %q is empty", inputPath)
	}
	if info.Size() > maxImageBytes {
		return tools.Result{}, fmt.Errorf("image path %q exceeds %d bytes", inputPath, maxImageBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{}, err
	}
	sourceMimeType, err := imageMimeType(data)
	if err != nil {
		return tools.Result{}, err
	}
	sourceSHA := sha256Hex(data)
	prompt, err := preparePromptImage(path, data, sourceMimeType)
	if err != nil {
		return tools.Result{}, err
	}
	promptSHA := sha256Hex(prompt.Data)
	blobPath, err := snapshotBlob(workspace, promptSHA, prompt.Data)
	if err != nil {
		return tools.Result{}, err
	}
	displayPath := displayPath(workspace, path, inputPath)
	filename := filepath.Base(displayPath)
	ref := media.Ref{
		Type:         media.TypeInputImage,
		Text:         "Image returned by view_image: " + displayPath,
		BlobPath:     blobPath,
		OriginalPath: displayPath,
		MimeType:     prompt.MimeType,
		Size:         int64(len(prompt.Data)),
		SHA256:       promptSHA,
		Detail:       detail,
		Filename:     filename,
		Width:        prompt.Width,
		Height:       prompt.Height,
	}

	content, err := json.Marshal(map[string]any{
		"status":             "ok",
		"message":            "Image attached for visual inspection.",
		"path":               displayPath,
		"mime_type":          sourceMimeType,
		"size":               len(data),
		"sha256":             sourceSHA,
		"prompt_mime_type":   prompt.MimeType,
		"prompt_size":        len(prompt.Data),
		"prompt_sha256":      promptSHA,
		"prompt_width":       prompt.Width,
		"prompt_height":      prompt.Height,
		"prompt_was_resized": prompt.Resized,
	})
	if err != nil {
		return tools.Result{}, err
	}

	return tools.Result{
		Content:   string(content),
		MediaRefs: []media.Ref{ref},
		Metadata: map[string]any{
			"path":               displayPath,
			"mime_type":          sourceMimeType,
			"size":               len(data),
			"sha256":             sourceSHA,
			"prompt_mime_type":   prompt.MimeType,
			"prompt_size":        len(prompt.Data),
			"prompt_sha256":      promptSHA,
			"prompt_width":       prompt.Width,
			"prompt_height":      prompt.Height,
			"prompt_was_resized": prompt.Resized,
		},
	}, nil
}

func imageDetail(detail string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(detail)) {
	case "":
		return "high", nil
	case "auto":
		return "auto", nil
	case "low":
		return "low", nil
	case "high":
		return "high", nil
	case "original":
		return "", errors.New("detail original is not supported by the native chat-completions image path; use high")
	default:
		return "", fmt.Errorf("unknown detail %q; valid values are auto, low, high", detail)
	}
}

type promptImage struct {
	Data     []byte
	MimeType string
	Width    int
	Height   int
	Resized  bool
}

func preparePromptImage(path string, data []byte, mimeType string) (promptImage, error) {
	decoded, err := decodeImage(data, mimeType)
	if err != nil {
		return promptImage{}, fmt.Errorf("unable to decode image at %q: %w", path, err)
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return promptImage{}, fmt.Errorf("image at %q has invalid dimensions %dx%d", path, width, height)
	}

	if width <= maxPromptImageWidth && height <= maxPromptImageHeight && canPreserveSourceBytes(mimeType) {
		return promptImage{
			Data:     data,
			MimeType: mimeType,
			Width:    width,
			Height:   height,
		}, nil
	}

	resized := decoded
	wasResized := false
	if width > maxPromptImageWidth || height > maxPromptImageHeight {
		resized = resizeToFit(decoded, maxPromptImageWidth, maxPromptImageHeight)
		wasResized = true
	}
	outputData, outputMimeType, err := encodePromptImage(resized, mimeType)
	if err != nil {
		return promptImage{}, err
	}
	outputBounds := resized.Bounds()
	return promptImage{
		Data:     outputData,
		MimeType: outputMimeType,
		Width:    outputBounds.Dx(),
		Height:   outputBounds.Dy(),
		Resized:  wasResized,
	}, nil
}

func decodeImage(data []byte, mimeType string) (image.Image, error) {
	reader := bytes.NewReader(data)
	switch mimeType {
	case "image/png":
		return png.Decode(reader)
	case "image/jpeg":
		return jpeg.Decode(reader)
	case "image/gif":
		return gif.Decode(reader)
	case "image/webp":
		return webp.Decode(reader)
	default:
		return nil, fmt.Errorf("unsupported image type %q", mimeType)
	}
}

func resizeToFit(src image.Image, maxWidth, maxHeight int) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scale := math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	nextWidth := max(1, min(maxWidth, int(math.Round(float64(width)*scale))))
	nextHeight := max(1, min(maxHeight, int(math.Round(float64(height)*scale))))
	dst := image.NewRGBA(image.Rect(0, 0, nextWidth, nextHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func encodePromptImage(img image.Image, sourceMimeType string) ([]byte, string, error) {
	var out bytes.Buffer
	switch sourceMimeType {
	case "image/jpeg":
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", fmt.Errorf("unable to encode resized jpeg: %w", err)
		}
		return out.Bytes(), "image/jpeg", nil
	default:
		if err := png.Encode(&out, img); err != nil {
			return nil, "", fmt.Errorf("unable to encode prompt png: %w", err)
		}
		return out.Bytes(), "image/png", nil
	}
}

func canPreserveSourceBytes(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func imageMimeType(data []byte) (string, error) {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png", nil
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg", nil
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif", nil
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp", nil
	default:
		return "", errors.New("unsupported image type; supported types are png, jpeg, gif, webp")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func snapshotBlob(workspace, sha string, data []byte) (string, error) {
	dir, err := filepath.Abs(filepath.Join(workspace, ".jaz-media", "blobs"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, sha)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if n, writeErr := file.Write(data); writeErr != nil {
			_ = file.Close()
			return "", writeErr
		} else if n != len(data) {
			_ = file.Close()
			return "", fmt.Errorf("short write for image blob %s: wrote %d of %d bytes", sha, n, len(data))
		}
		return path, file.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return "", err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(existing)
	if hex.EncodeToString(sum[:]) != sha {
		return "", fmt.Errorf("existing image blob hash mismatch for %s", sha)
	}
	return path, nil
}

func displayPath(workspace, path, input string) string {
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.ToSlash(strings.TrimSpace(input))
	}
	rel, err := filepath.Rel(workspaceAbs, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return filepath.ToSlash(strings.TrimSpace(input))
	}
	return filepath.ToSlash(rel)
}
