package app

import (
	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/connections/materialize"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/sourcequeue"
)

type MemorySourceQueue struct {
	*sourcequeue.Queue
}

type SourceProjectionQueue struct {
	*sourcequeue.Queue
}

func NewIntegrationRawWriter(layout runtimefiles.Layout) integrationingest.RawWriter {
	return integrationingest.RawWriter{Root: layout.Ingest}
}

func NewMemorySourceQueue(memory *jazmem.Memory) MemorySourceQueue {
	return MemorySourceQueue{Queue: sourcequeue.New(memory.Root())}
}

func NewSourceProjectionQueue(layout runtimefiles.Layout) SourceProjectionQueue {
	return SourceProjectionQueue{Queue: &sourcequeue.Queue{
		Root:      layout.Root,
		StateFile: ".state/source-projection.json",
	}}
}

func NewIntegrationMaterializingWriter(layout runtimefiles.Layout, raw integrationingest.RawWriter, queue SourceProjectionQueue, logger *log.Logger) integrationingest.MaterializingWriter {
	return integrationingest.MaterializingWriter{
		Raw:             raw,
		ProjectionQueue: queue.Queue,
		Log:             logger,
		Projector: integrationingest.SourceProjector{
			RawRoot:   layout.Ingest,
			StateRoot: layout.Root,
			Projector: integrationingest.CompositeSourceProjector(materialize.DefaultSourceProjectors()),
		},
	}
}

func NewSourceProjectionRunner(layout runtimefiles.Layout, memory *jazmem.Memory, projection SourceProjectionQueue, memoryQueue MemorySourceQueue) integrationingest.SourceProjectionRunner {
	return integrationingest.SourceProjectionRunner{
		Queue: projection.Queue,
		Projector: integrationingest.SourceProjector{
			RawRoot:   layout.Ingest,
			StateRoot: layout.Root,
			Projector: integrationingest.CompositeSourceProjector(materialize.DefaultSourceProjectors()),
		},
		Writer: integrationingest.SourceWriter{
			Root:             memory.Root(),
			DirtySourceStore: memoryQueue.Queue,
		},
	}
}
