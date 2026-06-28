package integrationingest

import (
	"context"

	"github.com/wins/jaz/backend/internal/sourcequeue"
)

const DefaultProjectionBatchFiles = 20

type SourceProjectionRunner struct {
	Queue      *sourcequeue.Queue
	Projector  SourceProjector
	Writer     SourceWriter
	BatchFiles int
}

func (r SourceProjectionRunner) RunOnce(ctx context.Context) (int, error) {
	if r.Queue == nil {
		return 0, nil
	}
	sources, err := r.Queue.Reserve(ctx, r.batchFiles())
	if err != nil || len(sources) == 0 {
		return 0, err
	}
	artifacts, err := r.Projector.ProjectSources(ctx, sources)
	if err != nil {
		_ = r.Queue.Release(context.Background(), sources)
		return 0, err
	}
	if err := r.Writer.WriteArtifacts(ctx, artifacts); err != nil {
		_ = r.Queue.Release(context.Background(), sources)
		return 0, err
	}
	if err := r.Queue.Complete(ctx, sources); err != nil {
		return 0, err
	}
	return len(sources), nil
}

func (r SourceProjectionRunner) batchFiles() int {
	if r.BatchFiles > 0 {
		return r.BatchFiles
	}
	return DefaultProjectionBatchFiles
}
