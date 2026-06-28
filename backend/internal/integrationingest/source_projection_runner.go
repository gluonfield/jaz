package integrationingest

import (
	"context"
	"errors"

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
	var completed []sourcequeue.Source
	var failed []sourcequeue.Source
	var firstErr error
	for _, source := range sources {
		artifacts, err := r.Projector.ProjectSource(ctx, source)
		if err == nil {
			err = r.Writer.WriteArtifacts(ctx, artifacts)
		}
		if err != nil {
			firstErr = errors.Join(firstErr, err)
			failed = append(failed, source)
			continue
		}
		completed = append(completed, source)
	}
	settleErr := r.Queue.Settle(context.Background(), completed, failed)
	if settleErr != nil {
		return len(completed), errors.Join(firstErr, settleErr)
	}
	if firstErr != nil {
		return len(completed), firstErr
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return len(completed), nil
}

func (r SourceProjectionRunner) batchFiles() int {
	if r.BatchFiles > 0 {
		return r.BatchFiles
	}
	return DefaultProjectionBatchFiles
}
