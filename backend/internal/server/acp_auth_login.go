package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/processenv"
)

const acpAuthLoginTimeout = 16 * time.Minute

type acpAuthLoginRequest struct {
	Auth acp.AgentAuthConfig `json:"auth,omitempty"`
}

type acpAuthLoginResponse struct {
	ID         string `json:"id"`
	Agent      string `json:"agent"`
	Status     string `json:"status"`
	Output     string `json:"output,omitempty"`
	AuthURL    string `json:"auth_url,omitempty"`
	AuthCode   string `json:"auth_code,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type acpAuthLoginJob struct {
	mu         sync.Mutex
	ID         string
	Agent      string
	Status     string
	Output     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	// Open while the login process runs, so the user can hand back a code the
	// browser printed (the remote/headless flow, where the CLI can't capture an
	// OAuth redirect on the user's machine).
	stdin io.WriteCloser
}

func (s *Server) handleStartACPAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	agent := acp.CanonicalAgentName(r.PathValue("agent"))
	cfg, ok := s.acpAgentCatalog().Agent(agent)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown acp agent %q", agent))
		return
	}
	var input acpAuthLoginRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			if !errors.Is(err, io.EOF) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}
	}
	auth, err := acp.LoginAuthConfig(agent, input.Auth)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg.AdapterBinDir = s.adapterBundleDir(cfg.ManagedAdapter)
	cfg.LoginBinDir = s.agentLoginBinDirs(cfg)
	probeCfg := cfg
	probeCfg.Auth = auth
	auth = acp.ProbeAgentAuth(agent, probeCfg, s.runtimeRoot(), nil).RecommendedAuth
	invocation := acp.AgentLoginInvocationFor(agent, s.runtimeRoot(), auth, cfg.LoginBinDir)
	if !invocation.Available {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%s", invocation.Reason))
		return
	}
	job, err := s.runACPAuthLogin(r.Context(), agent, auth, invocation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, job.response())
}

func (s *Server) handleGetACPAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/acp/auth-logins/"), "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("login id is required"))
		return
	}
	value, ok := s.acpAuthLoginJobs.Load(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("auth login not found"))
		return
	}
	job, ok := value.(*acpAuthLoginJob)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("auth login state is corrupt"))
		return
	}
	writeJSON(w, http.StatusOK, job.response())
}

func (s *Server) handleACPAuthLoginInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("login id is required"))
		return
	}
	value, ok := s.acpAuthLoginJobs.Load(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("auth login not found"))
		return
	}
	job, ok := value.(*acpAuthLoginJob)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("auth login state is corrupt"))
		return
	}
	var input struct {
		Input string `json:"input"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if strings.TrimSpace(input.Input) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("a code is required"))
		return
	}
	if err := job.writeInput(input.Input); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, job.response())
}

func (s *Server) runACPAuthLogin(ctx context.Context, agent string, auth acp.AgentAuthConfig, invocation acp.AgentLoginInvocation) (*acpAuthLoginJob, error) {
	id, err := newACPAuthLoginID()
	if err != nil {
		return nil, err
	}
	job := &acpAuthLoginJob{
		ID:        id,
		Agent:     agent,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	if err := acp.PrepareAgentLoginInvocation(agent, auth, invocation); err != nil {
		return nil, err
	}
	cmdCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acpAuthLoginTimeout)
	cmd := exec.CommandContext(cmdCtx, invocation.Executable, invocation.Args...)
	cmd.Env = acpAuthLoginEnv(invocation)
	if root := strings.TrimSpace(s.runtimeRoot()); root != "" {
		cmd.Dir = root
	}
	writer := acpAuthLoginWriter{job: job}
	if err := startACPAuthLoginProcess(cmd, job, writer, invocation.UsePTY); err != nil {
		cancel()
		return nil, err
	}
	var tail *acpAuthLogTail
	if log := strings.TrimSpace(invocation.TailLog); log != "" {
		tail = &acpAuthLogTail{path: log, w: writer}
		go tail.follow(cmdCtx)
	}
	s.acpAuthLoginJobs.Store(job.ID, job)
	go func() {
		defer cancel()
		err := cmd.Wait()
		// The process is done reading; stop accepting input before the (slower)
		// verify so a late paste rejects cleanly instead of writing to a pipe
		// Wait already closed.
		job.closeStdin()
		if tail != nil {
			tail.drain()
			_ = os.Remove(tail.path)
		}
		if err == nil {
			err = s.verifyACPAuthLogin(agent, auth)
		} else if last := job.lastOutputLine(); last != "" {
			// CLI exit codes are opaque; the last printed line (e.g. agy's
			// "Error: authentication failed or timed out") is the real story.
			err = errors.New(last)
		}
		job.finish(err, cmdCtx.Err())
	}()
	return job, nil
}

// startACPAuthLoginProcess wires the login process to the job. Keep stdin open
// so a code the user pastes back can reach a CLI that's blocked waiting for
// it; loopback flows simply never read it. usePTY runs the CLI on a
// pseudo-terminal for CLIs that stall draining piped stdin (agy).
func startACPAuthLoginProcess(cmd *exec.Cmd, job *acpAuthLoginJob, writer acpAuthLoginWriter, usePTY bool) error {
	if usePTY {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			return err
		}
		job.stdin = ptmx
		go func() {
			// The copy ends with EIO once the process exits and the pty closes.
			_, _ = io.Copy(writer, ptmx)
		}()
		return nil
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	job.stdin = stdin
	return cmd.Start()
}

var acpGlogLine = regexp.MustCompile(`^[IWEF][0-9]{4} `)

// acpAuthLogTail streams a CLI log file's human-facing lines into the login
// output: agy prints its sign-in URL and paste prompt only to its log, never
// to stdout.
type acpAuthLogTail struct {
	path string
	w    io.Writer

	mu     sync.Mutex
	offset int
}

func (t *acpAuthLogTail) follow(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.drain()
		}
	}
}

func (t *acpAuthLogTail) drain() {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := os.ReadFile(t.path)
	if err != nil {
		return
	}
	end := bytes.LastIndexByte(data, '\n') + 1
	if end <= t.offset {
		return
	}
	chunk := data[t.offset:end]
	t.offset = end
	for _, line := range strings.Split(string(chunk), "\n") {
		if strings.TrimSpace(line) == "" || acpGlogLine.MatchString(line) {
			continue
		}
		_, _ = fmt.Fprintln(t.w, line)
	}
}

func (s *Server) verifyACPAuthLogin(agent string, auth acp.AgentAuthConfig) error {
	cfg, _ := s.acpAgentCatalog().Agent(agent)
	cfg.Auth = auth
	cfg.AdapterBinDir = s.adapterBundleDir(cfg.ManagedAdapter)
	cfg.LoginBinDir = s.agentLoginBinDirs(cfg)
	status := acp.ProbeAgentAuthWithProviders(agent, cfg, s.runtimeRoot(), nil, s.modelProviders())
	if status.Authenticated {
		return nil
	}
	if strings.TrimSpace(status.Reason) != "" {
		return fmt.Errorf("%s", status.Reason)
	}
	return fmt.Errorf("sign-in finished but %s credentials were not saved", agent)
}

func newACPAuthLoginID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "login_" + hex.EncodeToString(b[:]), nil
}

func acpAuthLoginEnv(invocation acp.AgentLoginInvocation) []string {
	env := processenv.Base()
	processenv.PreserveHost(env, "LANG", "LC_ALL", "LC_CTYPE", "LOGNAME", "SHELL", "SSH_AUTH_SOCK", "USER")
	if invocation.UsePTY {
		processenv.PreserveHost(env, "TERM")
		if env["TERM"] == "" {
			env["TERM"] = "xterm-256color"
		}
	}
	if invocation.InheritHome {
		if home := os.Getenv("HOME"); strings.TrimSpace(home) != "" {
			env["HOME"] = home
		}
	}
	for key, value := range invocation.Env {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			env[key] = value
		}
	}
	return processenv.List(env)
}

func (j *acpAuthLoginJob) finish(runErr, ctxErr error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.FinishedAt = time.Now().UTC()
	switch {
	case ctxErr == context.DeadlineExceeded:
		j.Status = "failed"
		j.Error = "sign-in timed out; start sign-in again to get a fresh code"
	case runErr != nil:
		j.Status = "failed"
		j.Error = runErr.Error()
	default:
		j.Status = "succeeded"
	}
}

// lastOutputLine returns the final non-empty line the login process printed.
func (j *acpAuthLoginJob) lastOutputLine() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	clean := acpANSISequence.ReplaceAllString(j.Output, "")
	lines := strings.Split(clean, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// closeStdin stops the job from accepting input once its process has exited.
func (j *acpAuthLoginJob) closeStdin() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stdin != nil {
		_ = j.stdin.Close()
		j.stdin = nil
	}
}

// writeInput hands a line (a code the browser printed) to the login process's
// stdin so a CLI blocked waiting for it can finish.
func (j *acpAuthLoginJob) writeInput(input string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.Status != "running" || j.stdin == nil {
		return fmt.Errorf("sign-in is not waiting for a code")
	}
	line := strings.TrimRight(input, "\r\n") + "\n"
	if _, err := io.WriteString(j.stdin, line); err != nil {
		return fmt.Errorf("could not send the code to %s sign-in: %w", j.Agent, err)
	}
	return nil
}

func (j *acpAuthLoginJob) response() acpAuthLoginResponse {
	j.mu.Lock()
	defer j.mu.Unlock()
	authURL, authCode := acpAuthLoginHints(j.Output)
	res := acpAuthLoginResponse{
		ID:        j.ID,
		Agent:     j.Agent,
		Status:    j.Status,
		Output:    j.Output,
		AuthURL:   authURL,
		AuthCode:  authCode,
		Error:     j.Error,
		StartedAt: j.StartedAt.Format(time.RFC3339),
	}
	if !j.FinishedAt.IsZero() {
		res.FinishedAt = j.FinishedAt.Format(time.RFC3339)
	}
	return res
}

var (
	acpANSISequence = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	acpAuthURL      = regexp.MustCompile(`https://[^\s<>"']+`)
	acpAuthCode     = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{4,6}\b`)
)

func acpAuthLoginHints(output string) (string, string) {
	clean := acpANSISequence.ReplaceAllString(output, "")
	url := ""
	for _, match := range acpAuthURL.FindAllString(clean, -1) {
		match = strings.TrimRight(match, ".,)")
		if strings.Contains(match, "auth.") || strings.Contains(match, "claude.com") {
			url = match
			break
		}
		if url == "" {
			url = match
		}
	}
	code := acpAuthCode.FindString(clean)
	return url, code
}

type acpAuthLoginWriter struct {
	job *acpAuthLoginJob
}

func (w acpAuthLoginWriter) Write(p []byte) (int, error) {
	w.job.mu.Lock()
	defer w.job.mu.Unlock()
	w.job.Output += string(p)
	if len(w.job.Output) > 12000 {
		w.job.Output = w.job.Output[len(w.job.Output)-12000:]
	}
	return len(p), nil
}
