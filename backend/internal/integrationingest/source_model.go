package integrationingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func sourceFromTarget(target integrations.SourceTarget, pendingAt time.Time) sourcequeue.Source {
	return sourcequeue.Source{
		Path:      target.PathHint,
		PendingAt: pendingAt,
		Provider:  target.Provider,
		Kind:      target.Kind,
		MediaType: target.MediaType,
		Key:       target.Key,
		Replay:    target.Replay,
	}
}

func targetFromSource(source sourcequeue.Source) integrations.SourceTarget {
	mediaType := source.MediaType
	if mediaType == "" {
		mediaType = "text/markdown"
	}
	return integrations.SourceTarget{
		Provider:  source.Provider,
		Kind:      source.Kind,
		PathHint:  source.Path,
		MediaType: mediaType,
		Key:       source.Key,
		Replay:    source.Replay,
	}
}

func recordTime(record integrations.Record) time.Time {
	if !record.OccurredAt.IsZero() {
		return record.OccurredAt.UTC()
	}
	return record.ReceivedAt.UTC()
}

func cleanSourcePath(value string) (string, error) {
	value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	if value == "." || filepath.IsAbs(value) || value == ".." || strings.HasPrefix(value, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("source path escapes memory root")
	}
	return filepath.ToSlash(value), nil
}

func requiredStateRoot(value string) (string, error) {
	root := strings.TrimSpace(value)
	if root == "" {
		return "", fmt.Errorf("source projection state root is required")
	}
	return root, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
