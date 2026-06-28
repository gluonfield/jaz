package integrationingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type SourceProjector struct {
	RawRoot   string
	StateRoot string
	Projector integrations.SourceProjector
}

func (p SourceProjector) PlanRecords(ctx context.Context, records []integrations.Record) ([]sourcequeue.Source, error) {
	if p.Projector == nil {
		return nil, nil
	}
	seen := map[string]int{}
	var out []sourcequeue.Source
	var planErr error
	addSource := func(source sourcequeue.Source) {
		path, err := cleanSourcePath(source.Path)
		if err != nil {
			planErr = errors.Join(planErr, err)
			return
		}
		source.Path = path
		if index, ok := seen[path]; ok {
			if out[index].PendingAt.Before(source.PendingAt) {
				out[index] = source
			}
			return
		}
		seen[path] = len(out)
		out = append(out, source)
	}
	for _, record := range records {
		if record.Kind.Domain() == integrations.RecordDomainContacts {
			if source, ok := contactDependencySource(record); ok {
				addSource(source)
			}
		}
		targets, err := p.Projector.SourceTargets(ctx, integrations.MaterializeRequest{Record: record})
		if err != nil {
			planErr = errors.Join(planErr, err)
			continue
		}
		for _, target := range targets {
			addSource(sourceFromTarget(target, recordTime(record)))
			if err := p.recordSourceDependencies(target); err != nil {
				planErr = errors.Join(planErr, err)
			}
		}
	}
	return out, planErr
}

func (p SourceProjector) ProjectSource(ctx context.Context, source sourcequeue.Source) ([]integrations.Artifact, error) {
	if p.Projector == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source.Kind == sourceKindContactDependency {
		dependency, err := parseContactDependencyPath(source.Path)
		if err != nil {
			return nil, err
		}
		return p.projectContactDependency(ctx, dependency, source.PendingAt)
	}
	if source.Kind == "" {
		return nil, fmt.Errorf("source %q has no target kind", source.Path)
	}
	records, err := p.recordsForSource(source)
	if err != nil {
		return nil, err
	}
	artifact, err := p.Projector.ProjectSource(ctx, integrations.SourceProjectionRequest{
		Target:  targetFromSource(source),
		Records: records,
	})
	if err != nil {
		return nil, err
	}
	if artifact.PathHint == "" {
		return nil, fmt.Errorf("source projector produced no artifact for %q", source.Path)
	}
	return []integrations.Artifact{artifact}, nil
}
