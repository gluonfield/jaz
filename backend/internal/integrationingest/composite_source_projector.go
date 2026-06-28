package integrationingest

import (
	"context"
	"fmt"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type CompositeSourceProjector []integrations.SourceProjector

type providerSourceProjector interface {
	SourceProvider() string
}

func (p CompositeSourceProjector) SourceTargets(ctx context.Context, req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
	var out []integrations.SourceTarget
	for _, projector := range p {
		if projector == nil {
			continue
		}
		if provider := sourceProjectorProvider(projector); provider != "" && req.Record.Provider != "" && provider != req.Record.Provider {
			continue
		}
		targets, err := projector.SourceTargets(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, targets...)
	}
	return out, nil
}

func (p CompositeSourceProjector) ProjectSource(ctx context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	var matchedProvider bool
	for _, projector := range p {
		if projector == nil {
			continue
		}
		provider := sourceProjectorProvider(projector)
		if req.Target.Provider != "" {
			if provider == "" {
				continue
			}
			if provider != req.Target.Provider {
				continue
			}
			matchedProvider = true
		}
		artifact, err := projector.ProjectSource(ctx, req)
		if err != nil || artifact.PathHint != "" {
			return artifact, err
		}
	}
	if req.Target.Provider != "" && !matchedProvider {
		return integrations.Artifact{}, fmt.Errorf("no source projector for provider %q", req.Target.Provider)
	}
	return integrations.Artifact{}, nil
}

func sourceProjectorProvider(projector integrations.SourceProjector) string {
	owned, ok := projector.(providerSourceProjector)
	if !ok {
		return ""
	}
	return owned.SourceProvider()
}
