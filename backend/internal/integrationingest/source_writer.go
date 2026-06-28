package integrationingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type PendingSourceStore interface {
	MarkPendingSource(context.Context, sourcequeue.Source) error
}

type SourceWriter struct {
	Root string
	Now  func() time.Time
	PendingSourceStore
}

func (w SourceWriter) WriteArtifacts(ctx context.Context, artifacts []integrations.Artifact) error {
	for _, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.writeArtifact(ctx, artifact); err != nil {
			return err
		}
	}
	return nil
}

func (w SourceWriter) writeArtifact(ctx context.Context, artifact integrations.Artifact) error {
	root, err := requiredRoot(w.Root)
	if err != nil {
		return err
	}
	rel, path, err := sourceArtifactPath(root, artifact.PathHint)
	if err != nil {
		return err
	}
	if err := ensureSourceDir(root, filepath.Dir(path)); err != nil {
		return err
	}
	body := artifact.Body
	if len(body) > 0 {
		if body[len(body)-1] != '\n' {
			body = append(append([]byte{}, body...), '\n')
		}
	}
	if err := writeSourceFile(path, body); err != nil {
		return err
	}
	if w.PendingSourceStore == nil {
		return nil
	}
	return w.PendingSourceStore.MarkPendingSource(ctx, sourcequeue.Source{
		Path:      rel,
		PendingAt: w.now(),
		Provider:  artifact.Provider,
		Kind:      artifact.Kind,
		MediaType: artifact.MediaType,
	})
}

func sourceArtifactPath(root, hint string) (string, string, error) {
	hint = filepath.ToSlash(strings.TrimSpace(hint))
	if hint == "" {
		return "", "", fmt.Errorf("source artifact path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(hint))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("source artifact path escapes memory root")
	}
	path := filepath.Join(root, clean)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("source artifact path escapes memory root")
	}
	return filepath.ToSlash(rel), absPath, nil
}

func ensureSourceDir(root, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		return err
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("source artifact path escapes memory root")
	}
	return nil
}

func writeSourceFile(path string, body []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".source-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (w SourceWriter) now() time.Time {
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	return now
}
