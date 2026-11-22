package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/plugins/manifest"
	"github.com/wins/jaz/backend/internal/storage"
)

type RepositoryInput struct {
	Repository string   `json:"repository"`
	Ref        string   `json:"ref,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	PluginID   string   `json:"plugin_id,omitempty"`
	SourceID   string   `json:"source_id,omitempty"`
	AutoUpdate bool     `json:"auto_update,omitempty"`
}

type SkippedPlugin struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type InstallResult struct {
	Plugins []Plugin        `json:"plugins"`
	Skipped []SkippedPlugin `json:"skipped,omitempty"`
}

var githubRepository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var scpRepository = regexp.MustCompile(`^[^/@: ]+@[^/: ]+:.+$`)
var commitID = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

func repositoryURL(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n") || strings.HasPrefix(value, "ext::") {
		return "", fmt.Errorf("%w: provide a Git repository URL or GitHub owner/repo", ErrInvalid)
	}
	if strings.HasPrefix(value, "./") || filepath.IsAbs(value) {
		return filepath.Abs(value)
	}
	if githubRepository.MatchString(value) {
		return "https://github.com/" + strings.TrimSuffix(value, ".git") + ".git", nil
	}
	if scpRepository.MatchString(value) {
		if strings.ContainsAny(value, "?#") {
			return "", fmt.Errorf("%w: repository URLs must not contain query parameters", ErrInvalid)
		}
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%w: invalid Git repository URL", ErrInvalid)
	}
	password := false
	if parsed.User != nil {
		_, password = parsed.User.Password()
	}
	if parsed.Scheme != "" && (parsed.RawQuery != "" || parsed.Fragment != "" || password || parsed.User != nil && parsed.Scheme != "ssh") {
		return "", fmt.Errorf("%w: use the backend machine's Git credentials, without URL credentials or query parameters", ErrInvalid)
	}
	if parsed.Scheme == "" && !strings.Contains(value, ":") {
		return "", fmt.Errorf("%w: provide a Git URL, absolute local path or GitHub owner/repo", ErrInvalid)
	}
	return value, nil
}

func sourceIDFor(repoURL, ref string) string {
	id := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(repoURL), ".git"))
	id = strings.TrimPrefix(id, "https://github.com/")
	if ref != "" {
		id += "@" + ref
	}
	return id
}

func gitCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
	flags := []string{"-c", "http.userAgent=Mozilla/5.0", "-c", "credential.interactive=never", "-c", "core.hooksPath=" + os.DevNull, "-c", "protocol.ext.allow=never"}
	command := exec.CommandContext(ctx, "git", append(flags, args...)...)
	command.Dir = dir
	ssh := os.Getenv("GIT_SSH_COMMAND")
	if ssh == "" {
		ssh = "ssh"
	}
	command.Env = append(command.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GCM_GUI_PROMPT=0", "GIT_ASKPASS=false", "SSH_ASKPASS=false", "SSH_ASKPASS_REQUIRE=never", "GIT_SSH_COMMAND="+ssh+" -o BatchMode=yes")
	command.Stderr = io.Discard
	return command.Output()
}

func RemoteCommit(ctx context.Context, repoURL, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target := "HEAD"
	if ref = strings.TrimSpace(ref); ref != "" {
		if commitID.MatchString(ref) {
			return ref, nil
		}
		target = ref
	}
	raw, err := gitCommand(ctx, "", "ls-remote", "--", repoURL, target)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 || !commitID.MatchString(fields[0]) {
		return "", fmt.Errorf("remote ref %q not found", target)
	}
	return fields[0], nil
}

func (s *Store) cloneCheckout(ctx context.Context, input RepositoryInput) (workDir, checkoutDir, repoURL, commit string, err error) {
	input.Repository = strings.TrimSpace(input.Repository)
	input.Ref = strings.TrimSpace(input.Ref)
	repoURL, err = repositoryURL(input.Repository)
	if err != nil {
		return "", "", "", "", err
	}
	if strings.HasPrefix(input.Ref, "-") || strings.ContainsAny(input.Ref, "\x00\r\n:") {
		return "", "", "", "", fmt.Errorf("%w: invalid Git ref", ErrInvalid)
	}
	if err = os.MkdirAll(s.dir, 0o755); err != nil {
		return "", "", "", "", err
	}
	workDir, err = os.MkdirTemp(s.dir, ".install-")
	if err != nil {
		return "", "", "", "", err
	}
	checkoutDir = filepath.Join(workDir, "repository")
	if _, err = gitCommand(ctx, "", "clone", "--quiet", "--no-checkout", "--no-hardlinks", "--", repoURL, checkoutDir); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", "", "", fmt.Errorf("Git repository unavailable; the backend machine needs access through its existing Git credentials")
	}
	ref := "HEAD"
	if input.Ref != "" {
		if _, err = gitCommand(ctx, checkoutDir, "check-ref-format", "--allow-onelevel", input.Ref); err != nil {
			_ = os.RemoveAll(workDir)
			return "", "", "", "", fmt.Errorf("%w: invalid Git ref", ErrInvalid)
		}
		if _, err = gitCommand(ctx, checkoutDir, "fetch", "--quiet", "--", "origin", input.Ref); err != nil {
			_ = os.RemoveAll(workDir)
			return "", "", "", "", fmt.Errorf("Git ref unavailable; check the ref and the backend machine's repository access")
		}
		ref = "FETCH_HEAD"
	}
	rawCommit, err := gitCommand(ctx, checkoutDir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	commit = strings.TrimSpace(string(rawCommit))
	if err != nil || !commitID.MatchString(commit) {
		_ = os.RemoveAll(workDir)
		return "", "", "", "", fmt.Errorf("Git repository has no readable commit")
	}
	if _, err = gitCommand(ctx, checkoutDir, "checkout", "--quiet", "--detach", commit); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", "", "", fmt.Errorf("could not check out Git commit")
	}
	return workDir, checkoutDir, repoURL, commit, nil
}

func (s *Store) InspectSource(ctx context.Context, input RepositoryInput) (storage.PluginSourceRecord, []MarketplaceEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	work, checkout, repoURL, commit, err := s.cloneCheckout(ctx, input)
	if err != nil {
		return storage.PluginSourceRecord{}, nil, err
	}
	defer os.RemoveAll(work)
	var entries []MarketplaceEntry
	isMarketplace := false
	if len(input.Paths) > 0 {
		for _, sub := range input.Paths {
			resolved, err := manifest.Resolve(checkout, sub)
			if err != nil {
				return storage.PluginSourceRecord{}, nil, fmt.Errorf("%w: path %q: %v", ErrInvalid, sub, err)
			}
			plugin, err := readPackage(resolved, s.dataDir, nil)
			if err != nil {
				return storage.PluginSourceRecord{}, nil, err
			}
			entries = append(entries, entryFromPlugin(plugin, strings.TrimSpace(input.Repository), sub, strings.TrimSpace(input.Ref)))
		}
	} else {
		_, discovered, _, mk, err := s.marketplace(checkout, strings.TrimSpace(input.Repository))
		if err != nil {
			return storage.PluginSourceRecord{}, nil, err
		}
		entries = discovered
		isMarketplace = mk
	}
	catalogJSON, err := json.Marshal(entries)
	if err != nil {
		return storage.PluginSourceRecord{}, nil, err
	}
	now := time.Now().UTC()
	record := storage.PluginSourceRecord{
		ID:            sourceIDFor(repoURL, strings.TrimSpace(input.Ref)),
		Repository:    strings.TrimSpace(input.Repository),
		URL:           repoURL,
		Ref:           strings.TrimSpace(input.Ref),
		Paths:         input.Paths,
		CommitSHA:     commit,
		AutoUpdate:    input.AutoUpdate,
		IsMarketplace: isMarketplace,
		CatalogJSON:   catalogJSON,
		CheckedAt:     now,
		UpdatedAt:     now,
	}
	return record, entries, nil
}

func (s *Store) InstallRepository(ctx context.Context, input RepositoryInput) (InstallResult, error) {
	return s.installRepository(ctx, input, strings.TrimSpace(input.PluginID), "")
}

func (s *Store) Update(ctx context.Context, pluginID string) (InstallResult, error) {
	if !manifest.SafeName(pluginID) {
		return InstallResult{}, fmt.Errorf("%w: invalid plugin name", ErrInvalid)
	}
	if s.stateStore == nil {
		return InstallResult{}, fmt.Errorf("%w: plugin state store unavailable", ErrInvalid)
	}
	state, ok, err := s.stateStore.GetPluginState(ctx, pluginID)
	if err != nil {
		return InstallResult{}, err
	}
	if !ok || state.SourceRepository == "" {
		return InstallResult{}, fmt.Errorf("%w: this plugin was not installed from Git", ErrInvalid)
	}
	var paths []string
	if state.SourcePath != "" {
		paths = []string{state.SourcePath}
	}
	return s.installRepository(ctx, RepositoryInput{
		Repository: state.SourceRepository,
		Ref:        state.SourceRef,
		Paths:      paths,
		SourceID:   state.SourceID,
	}, pluginID, state.CommitSHA)
}

func (s *Store) installRepository(ctx context.Context, input RepositoryInput, wanted, expectedCommit string) (result InstallResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	work, checkout, repoURL, commit, err := s.cloneCheckout(ctx, input)
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(work)

	var prepared []preparedPlugin
	var skipped []SkippedPlugin
	var entries []MarketplaceEntry
	if len(input.Paths) > 0 {
		for _, sub := range input.Paths {
			resolved, err := manifest.Resolve(checkout, sub)
			if err != nil {
				return result, fmt.Errorf("%w: path %q: %v", ErrInvalid, sub, err)
			}
			plugin, err := readPackage(resolved, s.dataDir, nil)
			if err != nil {
				return result, err
			}
			prepared = append(prepared, preparedPlugin{plugin: plugin, path: resolved})
		}
	} else {
		prepared, entries, skipped, _, err = s.marketplace(checkout, strings.TrimSpace(input.Repository))
		if err != nil {
			return result, err
		}
	}

	// If wanted refers to an external git pointer inside a marketplace, follow it once.
	if wanted != "" {
		var selected []preparedPlugin
		for _, candidate := range prepared {
			if candidate.plugin.ID == wanted {
				selected = append(selected, candidate)
			}
		}
		if len(selected) == 0 {
			for _, entry := range entries {
				if entry.ID == wanted && entry.Repository != "" && entry.Repository != strings.TrimSpace(input.Repository) {
					var subpaths []string
					if entry.Path != "" {
						subpaths = []string{entry.Path}
					}
					return s.installRepository(ctx, RepositoryInput{
						Repository: entry.Repository,
						Ref:        entry.Ref,
						Paths:      subpaths,
						SourceID:   input.SourceID,
					}, wanted, expectedCommit)
				}
			}
			return result, fmt.Errorf("%w: plugin %q not found in repository", ErrInvalid, wanted)
		}
		prepared = selected
	}

	if len(prepared) == 0 {
		return InstallResult{Plugins: []Plugin{}, Skipped: skipped}, nil
	}

	sourceID := input.SourceID
	if sourceID == "" {
		sourceID = sourceIDFor(repoURL, strings.TrimSpace(input.Ref))
	}
	for i := range prepared {
		relPath, _ := filepath.Rel(checkout, prepared[i].path)
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			relPath = ""
		}
		src := Source{
			ID:         sourceID,
			Repository: strings.TrimSpace(input.Repository),
			Ref:        strings.TrimSpace(input.Ref),
			Path:       relPath,
			Commit:     commit,
		}
		prepared[i].source = &src
		prepared[i].expected = expectedCommit
	}
	prepared, err = stagePackages(checkout, prepared, filepath.Join(work, "ready"), s.dataDir)
	if err != nil {
		return result, err
	}
	installed, err := s.publish(ctx, prepared)
	return InstallResult{Plugins: installed, Skipped: skipped}, err
}
