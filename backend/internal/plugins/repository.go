package plugins

import (
	"context"
	"errors"
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
	"github.com/wins/jaz/backend/internal/skills"
)

type RepositoryInput struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref,omitempty"`
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

func (s *Store) InstallRepository(ctx context.Context, input RepositoryInput) (InstallResult, error) {
	return s.installRepository(ctx, input, "", nil)
}

func (s *Store) Update(ctx context.Context, name string) (InstallResult, error) {
	if !manifest.SafeName(name) {
		return InstallResult{}, fmt.Errorf("%w: invalid plugin name", ErrInvalid)
	}
	s.mu.RLock()
	sources, err := s.sources()
	_, installedErr := os.Stat(filepath.Join(s.dir, name))
	s.mu.RUnlock()
	if err != nil {
		return InstallResult{}, err
	}
	if installedErr != nil {
		return InstallResult{}, ErrNotFound
	}
	source, ok := sources[name]
	if !ok {
		return InstallResult{}, fmt.Errorf("%w: this plugin was not installed from Git", ErrInvalid)
	}
	return s.installRepository(ctx, RepositoryInput{Repository: source.Repository, Ref: source.Ref}, name, &source)
}

func (s *Store) installRepository(ctx context.Context, input RepositoryInput, wanted string, expected *Source) (result InstallResult, err error) {
	input.Repository = strings.TrimSpace(input.Repository)
	input.Ref = strings.TrimSpace(input.Ref)
	repository, err := repositoryURL(input.Repository)
	if err != nil {
		return result, err
	}
	if strings.HasPrefix(input.Ref, "-") || strings.ContainsAny(input.Ref, "\x00\r\n:") {
		return result, fmt.Errorf("%w: invalid Git ref", ErrInvalid)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err = os.MkdirAll(s.dir, 0755); err != nil {
		return result, err
	}
	work, err := os.MkdirTemp(s.dir, ".install-")
	if err != nil {
		return result, err
	}
	defer func() {
		if !errors.Is(err, errKeepStaging) {
			_ = os.RemoveAll(work)
		}
	}()
	repo := filepath.Join(work, "repository")
	if _, err = gitCommand(ctx, "", "clone", "--quiet", "--no-checkout", "--no-hardlinks", "--", repository, repo); err != nil {
		return result, fmt.Errorf("Git repository unavailable; the backend machine needs access through its existing Git credentials")
	}
	ref := "HEAD"
	if input.Ref != "" {
		if _, err = gitCommand(ctx, repo, "check-ref-format", "--allow-onelevel", input.Ref); err != nil {
			return result, fmt.Errorf("%w: invalid Git ref", ErrInvalid)
		}
		if _, err = gitCommand(ctx, repo, "fetch", "--quiet", "--", "origin", input.Ref); err != nil {
			return result, fmt.Errorf("Git ref unavailable; check the ref and the backend machine's repository access")
		}
		ref = "FETCH_HEAD"
	}
	rawCommit, err := gitCommand(ctx, repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	commit := strings.TrimSpace(string(rawCommit))
	if err != nil || !commitID.MatchString(commit) {
		return result, fmt.Errorf("Git repository has no readable commit")
	}
	archive := filepath.Join(work, "repository.zip")
	if _, err = gitCommand(ctx, repo, "archive", "--format=zip", "--output="+archive, commit); err != nil {
		return result, fmt.Errorf("could not export Git repository")
	}
	info, err := os.Stat(archive)
	if err != nil || info.Size() > 64<<20 {
		return result, fmt.Errorf("%w: repository package exceeds 64 MiB", ErrInvalid)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return result, err
	}
	tree := filepath.Join(work, "content")
	if err = os.MkdirAll(tree, 0755); err != nil {
		return result, err
	}
	if err = skills.ExtractArchive(data, tree); err != nil {
		return result, fmt.Errorf("%w: repository package: %v", ErrInvalid, err)
	}
	prepared, skipped, err := s.marketplace(tree)
	if err != nil {
		return result, err
	}
	if wanted != "" {
		var selected []preparedPlugin
		for _, candidate := range prepared {
			if candidate.plugin.Name == wanted {
				selected = append(selected, candidate)
			}
		}
		prepared = selected
		if len(prepared) == 0 {
			return result, fmt.Errorf("%w: plugin is missing or invalid in the repository", ErrInvalid)
		}
	}
	source := Source{Repository: repository, Ref: input.Ref, Commit: commit}
	for i := range prepared {
		prepared[i].source = &source
		prepared[i].expected = expected
	}
	prepared, err = stagePackages(prepared, filepath.Join(work, "ready"))
	if err != nil {
		return result, err
	}
	installed, err := s.publish(prepared)
	return InstallResult{Plugins: installed, Skipped: skipped}, err
}
