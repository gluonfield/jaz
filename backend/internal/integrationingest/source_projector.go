package integrationingest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sourcequeue"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type SourceProjector struct {
	RawRoot   string
	StateRoot string
	Projector integrations.SourceProjector
}

type sourceAddress struct {
	Path     string
	Lane     string
	Provider string
	Account  string
	Slug     string
	Year     string
	Month    string
	Day      string
}

const sourceKindContactDependency = "contact_dependency"

type contactDependencyEntry struct {
	Path      string `json:"path"`
	Provider  string `json:"provider,omitempty"`
	Kind      string `json:"kind,omitempty"`
	MediaType string `json:"media_type,omitempty"`
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
			if out[index].DirtyAt.Before(source.DirtyAt) {
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

func contactDependencySource(record integrations.Record) (sourcequeue.Source, bool) {
	provider := integrations.NormalizeAlias(record.Provider)
	account := integrations.NormalizeAlias(record.AccountID)
	if provider == "" || account == "" || strings.TrimSpace(record.ExternalID) == "" {
		return sourcequeue.Source{}, false
	}
	path := filepath.ToSlash(filepath.Join(".state", "source-dependencies", "chat-contact", provider, account, integrations.SourceSlug(record.ExternalID)+".dep"))
	return sourcequeue.Source{Path: path, DirtyAt: recordTime(record), Provider: provider, Kind: sourceKindContactDependency}, true
}

func (p SourceProjector) projectContactDependency(ctx context.Context, dependency sourceAddress, dirtyAt time.Time) ([]integrations.Artifact, error) {
	dirty, err := p.contactDependencySources(dependency, dirtyAt)
	if err != nil {
		return nil, err
	}
	return p.ProjectSources(ctx, dirty)
}

func (p SourceProjector) recordSourceDependencies(target integrations.SourceTarget) error {
	if strings.TrimSpace(p.StateRoot) == "" || len(target.ContactRefs) == 0 {
		return nil
	}
	address, err := parseSourceAddress(target.PathHint)
	if err != nil {
		return err
	}
	entry := contactDependencyEntry{
		Path:      address.Path,
		Provider:  firstNonEmpty(target.Provider, address.Provider),
		Kind:      target.Kind,
		MediaType: target.MediaType,
	}
	for _, ref := range target.ContactRefs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		if err := p.appendContactDependency(address.Provider, address.Account, integrations.SourceSlug(ref), entry); err != nil {
			return err
		}
	}
	return nil
}

func (p SourceProjector) appendContactDependency(provider, account, contactSlug string, entry contactDependencyEntry) error {
	path, err := p.contactDependencyIndexPath(sourceAddress{Provider: provider, Account: account, Slug: contactSlug})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (p SourceProjector) contactDependencySources(dependency sourceAddress, dirtyAt time.Time) ([]sourcequeue.Source, error) {
	path, err := p.contactDependencyIndexPath(dependency)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	seen := map[string]sourcequeue.Source{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry contactDependencyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		sourcePath, err := cleanSourcePath(entry.Path)
		if err != nil {
			return nil, err
		}
		seen[sourcePath] = sourcequeue.Source{
			Path:      sourcePath,
			DirtyAt:   dirtyAt,
			Provider:  firstNonEmpty(entry.Provider, dependency.Provider),
			Kind:      entry.Kind,
			MediaType: entry.MediaType,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]sourcequeue.Source, 0, len(paths))
	for _, path := range paths {
		out = append(out, seen[path])
	}
	return out, nil
}

func (p SourceProjector) contactDependencyIndexPath(dependency sourceAddress) (string, error) {
	root, err := requiredStateRoot(p.StateRoot)
	if err != nil {
		return "", err
	}
	provider, err := requiredPathComponent("provider", dependency.Provider)
	if err != nil {
		return "", err
	}
	account, err := requiredPathComponent("account", dependency.Account)
	if err != nil {
		return "", err
	}
	contact, err := requiredPathComponent("contact", dependency.Slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".state", "source-dependencies", "chat-contact", provider, account, contact+".jsonl"), nil
}

func (p SourceProjector) ProjectSources(ctx context.Context, sources []sourcequeue.Source) ([]integrations.Artifact, error) {
	if p.Projector == nil || len(sources) == 0 {
		return nil, nil
	}
	artifacts := make([]integrations.Artifact, 0, len(sources))
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		address, err := parseSourceAddress(source.Path)
		if err != nil {
			return nil, err
		}
		if source.Kind == sourceKindContactDependency {
			dependentArtifacts, err := p.projectContactDependency(ctx, address, source.DirtyAt)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, dependentArtifacts...)
			continue
		}
		if source.Kind == "" {
			return nil, fmt.Errorf("source %q has no target kind", source.Path)
		}
		records, err := p.recordsForSource(address.Path)
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
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (p SourceProjector) recordsForSource(sourcePath string) ([]integrations.Record, error) {
	rawFiles, err := p.rawFilesForSource(sourcePath)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []integrations.Record
	for _, file := range rawFiles {
		records, err := readRawRecords(file)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			id := record.ID
			if id == "" {
				id = recordID(record)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := recordTime(out[i])
		right := recordTime(out[j])
		if left.Equal(right) {
			return out[i].ExternalID < out[j].ExternalID
		}
		return left.Before(right)
	})
	return out, nil
}

func (p SourceProjector) rawFilesForSource(sourcePath string) ([]string, error) {
	root, err := requiredRoot(p.RawRoot)
	if err != nil {
		return nil, err
	}
	address, err := parseSourceAddress(sourcePath)
	if err != nil {
		return nil, err
	}
	switch address.Lane {
	case "contacts":
		return []string{filepath.Join(root, address.Provider, address.Account, "contacts", "contacts.jsonl")}, nil
	case "conversations":
		return []string{
			filepath.Join(root, address.Provider, address.Account, "messages", address.Year, address.Month, address.Day, "messages.jsonl"),
			filepath.Join(root, address.Provider, address.Account, "contacts", "contacts.jsonl"),
		}, nil
	case "messages":
		return []string{filepath.Join(root, address.Provider, address.Account, "messages", address.Year, address.Month, address.Day, "messages.jsonl")}, nil
	default:
		return nil, fmt.Errorf("unsupported source path %q", sourcePath)
	}
}

func sourceFromTarget(target integrations.SourceTarget, dirtyAt time.Time) sourcequeue.Source {
	return sourcequeue.Source{
		Path:      target.PathHint,
		DirtyAt:   dirtyAt,
		Provider:  target.Provider,
		Kind:      target.Kind,
		MediaType: target.MediaType,
	}
}

func targetFromSource(source sourcequeue.Source) integrations.SourceTarget {
	mediaType := source.MediaType
	if mediaType == "" {
		mediaType = "text/markdown"
	}
	return integrations.SourceTarget{Provider: source.Provider, Kind: source.Kind, PathHint: source.Path, MediaType: mediaType}
}

func parseSourceAddress(sourcePath string) (sourceAddress, error) {
	path, err := cleanSourcePath(sourcePath)
	if err != nil {
		return sourceAddress{}, err
	}
	parts := strings.Split(path, "/")
	switch {
	case len(parts) == 4 && parts[0] == "sources" && parts[3] == "contacts.md":
		return sourceAddress{Path: path, Lane: "contacts", Provider: parts[1], Account: parts[2]}, nil
	case len(parts) == 8 && parts[0] == "sources" && parts[3] == "conversations" && strings.HasSuffix(parts[7], ".md"):
		return sourceAddress{Path: path, Lane: "conversations", Provider: parts[1], Account: parts[2], Slug: parts[4], Year: parts[5], Month: parts[6], Day: strings.TrimSuffix(parts[7], ".md")}, nil
	case len(parts) == 8 && parts[0] == "sources" && parts[3] == "messages" && strings.HasSuffix(parts[7], ".md"):
		return sourceAddress{Path: path, Lane: "messages", Provider: parts[1], Account: parts[2], Slug: strings.TrimSuffix(parts[7], ".md"), Year: parts[4], Month: parts[5], Day: parts[6]}, nil
	case len(parts) == 6 && parts[0] == ".state" && parts[1] == "source-dependencies" && parts[2] == "chat-contact" && strings.HasSuffix(parts[5], ".dep"):
		return sourceAddress{Path: path, Lane: "dependency", Provider: parts[3], Account: parts[4], Slug: strings.TrimSuffix(parts[5], ".dep")}, nil
	default:
		return sourceAddress{}, fmt.Errorf("unsupported source path %q", sourcePath)
	}
}

func readRawRecords(path string) ([]integrations.Record, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var records []integrations.Record
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var record integrations.Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, err
		}
		if record.Kind != "" {
			records = append(records, record)
		}
	}
	return records, scanner.Err()
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
