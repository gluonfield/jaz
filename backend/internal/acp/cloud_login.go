package acp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/processenv"
)

const isolatedCloudLoginTimeout = 10 * time.Minute

var (
	ErrUnsupportedCloudLoginAgent = errors.New("isolated desktop OAuth login is not supported for this agent")
	ErrIsolatedAuthNotCaptured    = errors.New("sign-in finished without saving credentials")
	ErrCloudLoginNotFound         = errors.New("login session not found")
)

type CloudLoginSnapshot struct {
	ID              string `json:"id"`
	Agent           string `json:"agent"`
	Status          string `json:"status"`
	VerificationURL string `json:"verification_url,omitempty"`
	UserCode        string `json:"user_code,omitempty"`
	Error           string `json:"error,omitempty"`
	ExpiresAt       string `json:"expires_at"`
}

type IsolatedCloudLogin struct {
	Agent      string
	Dir        string
	Invocation AgentLoginInvocation
	Cleanup    func()
}

type CloudLoginCommitFunc func(ctx context.Context, agent string, files map[string][]byte) error

type cloudLoginRecord struct {
	mu              sync.Mutex
	id              string
	agent           string
	status          string
	verificationURL string
	userCode        string
	errMsg          string
	expiresAt       time.Time
	cancel          context.CancelFunc
}

func (r *cloudLoginRecord) snapshot() CloudLoginSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CloudLoginSnapshot{
		ID:              r.id,
		Agent:           r.agent,
		Status:          r.status,
		VerificationURL: r.verificationURL,
		UserCode:        r.userCode,
		Error:           r.errMsg,
		ExpiresAt:       r.expiresAt.UTC().Format(time.RFC3339),
	}
}

// CloudLoginRunner owns isolated Desktop OAuth login processes for Cloud
// workspaces. Each login runs inside an ephemeral 0700 directory so Local
// workspace credentials are never read, shared, or modified.
type CloudLoginRunner struct {
	prepare func(agent string) (IsolatedCloudLogin, error)
	capture func(agent, dir string) (map[string][]byte, error)

	mu         sync.Mutex
	byID       map[string]*cloudLoginRecord
	activeByAg map[string]*cloudLoginRecord
}

func NewCloudLoginRunner(prepare func(agent string) (IsolatedCloudLogin, error)) *CloudLoginRunner {
	return &CloudLoginRunner{
		prepare:    prepare,
		capture:    CaptureIsolatedAuthFiles,
		byID:       make(map[string]*cloudLoginRecord),
		activeByAg: make(map[string]*cloudLoginRecord),
	}
}

func (m *Manager) StartIsolatedCloudLogin(agent string) (IsolatedCloudLogin, error) {
	name := CanonicalAgentName(agent)
	var cfg AgentConfig
	if m.agents != nil {
		if found, ok, err := m.agents.AgentConfig(name); err == nil && ok {
			cfg = found
		}
	}
	return PrepareIsolatedCloudLogin(name, cfg, m.cfg.Root)
}

func (r *CloudLoginRunner) Start(agent string, commit CloudLoginCommitFunc) (CloudLoginSnapshot, error) {
	iso, err := r.prepare(agent)
	if err != nil {
		return CloudLoginSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), isolatedCloudLoginTimeout)
	cmd := exec.CommandContext(ctx, iso.Invocation.Executable, iso.Invocation.Args...)
	cmd.Dir = iso.Dir
	cmd.Env = isolatedCloudLoginEnv(iso.Invocation)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		cancel()
		_ = pw.Close()
		_ = pr.Close()
		iso.Cleanup()
		return CloudLoginSnapshot{}, err
	}

	rec := &cloudLoginRecord{
		id:        newCloudLoginID(),
		agent:     iso.Agent,
		status:    "pending",
		expiresAt: time.Now().UTC().Add(isolatedCloudLoginTimeout),
		cancel:    cancel,
	}

	// Cancel any previous in-flight login for the same agent so concurrent attempts
	// never fight over the browser or leak orphan CLI processes.
	r.mu.Lock()
	prev := r.activeByAg[iso.Agent]
	r.activeByAg[iso.Agent] = rec
	r.byID[rec.id] = rec
	r.mu.Unlock()
	if prev != nil && prev.cancel != nil {
		prev.cancel()
	}

	go r.watch(ctx, cancel, cmd, pr, pw, iso, rec, commit)
	return rec.snapshot(), nil
}

func (r *CloudLoginRunner) Get(agent, id string) (CloudLoginSnapshot, error) {
	norm := CanonicalAgentName(agent)
	r.mu.Lock()
	rec := r.byID[strings.TrimSpace(id)]
	r.mu.Unlock()
	if rec == nil || rec.agent != norm {
		return CloudLoginSnapshot{}, ErrCloudLoginNotFound
	}
	return rec.snapshot(), nil
}

func (r *CloudLoginRunner) watch(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	pr *io.PipeReader,
	pw *io.PipeWriter,
	iso IsolatedCloudLogin,
	rec *cloudLoginRecord,
	commit CloudLoginCommitFunc,
) {
	defer cancel()
	defer iso.Cleanup()
	defer func() {
		r.mu.Lock()
		if r.activeByAg[rec.agent] == rec {
			delete(r.activeByAg, rec.agent)
		}
		r.mu.Unlock()
		time.AfterFunc(5*time.Minute, func() {
			r.mu.Lock()
			delete(r.byID, rec.id)
			r.mu.Unlock()
		})
	}()

	var tail strings.Builder
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 4096), 64*1024)
		for scanner.Scan() {
			line := cloudLoginANSIRe.ReplaceAllString(scanner.Text(), "")
			if tail.Len() < 4096 {
				tail.WriteString(line)
				tail.WriteByte('\n')
			}
			u := strings.TrimRight(cloudLoginURLRe.FindString(line), ".,);:")
			c := cloudLoginCodeRe.FindString(line)
			if u != "" || c != "" {
				rec.mu.Lock()
				if u != "" && rec.verificationURL == "" {
					rec.verificationURL = u
				}
				if c != "" && rec.userCode == "" {
					rec.userCode = c
				}
				if rec.status == "pending" {
					rec.status = "awaiting_user"
				}
				rec.mu.Unlock()
			}
		}
	}()

	waitErr := cmd.Wait()
	_ = pw.Close()
	<-scanDone

	if waitErr != nil {
		msg := strings.TrimSpace(tail.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		rec.mu.Lock()
		switch ctx.Err() {
		case context.DeadlineExceeded:
			rec.status = "expired"
			rec.errMsg = "sign-in timed out"
		case context.Canceled:
			rec.status = "failed"
			rec.errMsg = "sign-in was replaced by a newer attempt"
		default:
			rec.status = "failed"
			rec.errMsg = msg
		}
		rec.mu.Unlock()
		return
	}

	files, err := r.capture(iso.Agent, iso.Dir)
	if err != nil {
		rec.mu.Lock()
		rec.status = "failed"
		rec.errMsg = err.Error()
		rec.mu.Unlock()
		return
	}
	commitCtx, commitCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer commitCancel()
	if err := commit(commitCtx, iso.Agent, files); err != nil {
		rec.mu.Lock()
		rec.status = "failed"
		rec.errMsg = err.Error()
		rec.mu.Unlock()
		return
	}
	rec.mu.Lock()
	rec.status = "completed"
	rec.errMsg = ""
	rec.mu.Unlock()
}

func PrepareIsolatedCloudLogin(agent string, cfg AgentConfig, root string) (IsolatedCloudLogin, error) {
	name := CanonicalAgentName(agent)
	switch name {
	case AgentClaude, AgentKimi:
	default:
		return IsolatedCloudLogin{}, fmt.Errorf("%w: %s", ErrUnsupportedCloudLoginAgent, agent)
	}
	dir, err := os.MkdirTemp("", "jaz-cloud-acp-*")
	if err != nil {
		return IsolatedCloudLogin{}, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return IsolatedCloudLogin{}, err
	}
	auth := AgentAuthConfig{Mode: AuthModeJazProfile, Path: dir}
	inv := AgentLoginInvocationFor(name, root, auth, loginBinDirs(cfg), cfg.Env)
	if !inv.Available {
		_ = os.RemoveAll(dir)
		reason := strings.TrimSpace(inv.Reason)
		if reason == "" {
			reason = fmt.Sprintf("%s CLI is not installed locally", name)
		}
		return IsolatedCloudLogin{}, errors.New(reason)
	}
	cleanup := func() {
		if name == AgentClaude {
			_ = removeClaudeProfileKeychainCredential(dir)
		}
		_ = os.RemoveAll(dir)
	}
	return IsolatedCloudLogin{
		Agent:      name,
		Dir:        dir,
		Invocation: inv,
		Cleanup:    cleanup,
	}, nil
}

// CaptureIsolatedAuthFiles reads credential files written inside an isolated
// login directory (including macOS Keychain fallback for Claude Code).
func CaptureIsolatedAuthFiles(agent, dir string) (map[string][]byte, error) {
	name := CanonicalAgentName(agent)
	files := make(map[string][]byte)
	switch name {
	case AgentClaude:
		if data, ok := readIsolatedClaudeCredentials(dir); ok {
			files[".credentials.json"] = data
		}
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, ".claude.json")); ok {
			files[".claude.json"] = data
		}
		if len(files[".credentials.json"]) == 0 {
			return nil, fmt.Errorf("%w for claude", ErrIsolatedAuthNotCaptured)
		}
	case AgentKimi:
		if data, ok := readRegularNonEmptyFile(filepath.Join(dir, "config.toml")); ok {
			files["config.toml"] = data
		}
		credDir := filepath.Join(dir, "credentials")
		if entries, err := os.ReadDir(credDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				if data, ok := readRegularNonEmptyFile(filepath.Join(credDir, entry.Name())); ok {
					files["credentials/"+entry.Name()] = data
				}
			}
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("%w for kimi", ErrIsolatedAuthNotCaptured)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedCloudLoginAgent, agent)
	}
	return files, nil
}

func isolatedCloudLoginEnv(inv AgentLoginInvocation) []string {
	env := processenv.Base()
	processenv.PreserveHost(env, "HTTP_PROXY", "HTTPS_PROXY", "LANG", "LC_ALL", "LC_CTYPE", "LOGNAME", "NO_PROXY", "SHELL", "USER")
	if inv.InheritHome {
		if home := os.Getenv("HOME"); strings.TrimSpace(home) != "" {
			env["HOME"] = home
		}
	}
	for k, v := range inv.Env {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			env[k] = v
		}
	}
	return processenv.List(env)
}

func newCloudLoginID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "dlog_" + hex.EncodeToString(b[:])
}

func readRegularNonEmptyFile(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func readIsolatedClaudeCredentials(configDir string) ([]byte, bool) {
	if data, ok := readRegularNonEmptyFile(filepath.Join(configDir, ".credentials.json")); ok {
		if tok, valid := claudeAccessToken(data); valid && tok != "" {
			return data, true
		}
	}
	if runtime.GOOS != "darwin" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeCredentialReadLimit)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", claudeKeychainService(configDir), "-w").Output()
	if err != nil {
		return nil, false
	}
	if tok, valid := claudeAccessToken(out); valid && tok != "" {
		return []byte(strings.TrimSpace(string(out))), true
	}
	return nil, false
}

var (
	cloudLoginANSIRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	cloudLoginURLRe  = regexp.MustCompile(`https://[^\s"'<>]+`)
	cloudLoginCodeRe = regexp.MustCompile(`\b[A-Z0-9]{4,5}-[A-Z0-9]{4,5}\b`)
)
