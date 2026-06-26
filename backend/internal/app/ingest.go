package app

import (
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/runtimefiles"
)

func NewIntegrationRawWriter(layout runtimefiles.Layout) integrationingest.RawWriter {
	return integrationingest.RawWriter{Root: layout.Ingest}
}
