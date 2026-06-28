package integrationingest

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type MaterializingWriter struct {
	Raw             RawWriter
	Projector       SourceProjector
	ProjectionQueue DirtySourceStore
	Log             *log.Logger
}

func (w MaterializingWriter) WriteRecords(ctx context.Context, records []integrations.Record) error {
	prepared := make([]integrations.Record, 0, len(records))
	for _, record := range records {
		prepared = append(prepared, w.Raw.prepare(record))
	}
	if err := w.Raw.WriteRecords(ctx, prepared); err != nil {
		return err
	}
	sources, err := w.Projector.PlanRecords(ctx, prepared)
	if err != nil {
		w.warn("source projection planning failed", err)
	}
	for _, source := range sources {
		if err := w.markProjectionDirty(ctx, source); err != nil {
			w.warn("source projection queue update failed", err)
		}
	}
	return nil
}

func (w MaterializingWriter) markProjectionDirty(ctx context.Context, source sourcequeue.Source) error {
	if w.ProjectionQueue == nil {
		return nil
	}
	return w.ProjectionQueue.MarkDirtySource(ctx, source)
}

func (w MaterializingWriter) warn(message string, err error) {
	if err != nil && w.Log != nil {
		w.Log.WithPrefix("integration-ingest").Warn(message, "error", err)
	}
}
