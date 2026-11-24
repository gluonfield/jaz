package server

import (
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
	m365copilotapi "github.com/wins/jaz/backend/internal/httpapi/m365copilot"
	"github.com/wins/jaz/backend/internal/processenv"
)

const acpAuthLoginTimeout = 16 * time.Minute

var errAntigravityAuthBusy = errors.New("another Antigravity account operation is in progress")

func (s *Server) claimACPAuth(agent string) (func(), error) {
	if agent != acp.AgentAntigravity {
		return func() {}, nil
	}
	if !s.antigravityLoginMu.TryLock() {
		return nil, errAntigravityAuthBusy
	}
	return sync.OnceFunc(s.antigravityLoginMu.Unlock), nil
}

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
	authURL    string
	onInput    func(string) error
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
	if agent == acp.AgentMicrosoftCopilot {
		session, err := s.MicrosoftCopilot.StartLogin(r.Context(), m365copilotapi.CallbackURL(r, s.ServerConfig.PublicBaseURL()))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		job, err := s.startACPOAuthLogin(agent, auth, session.AuthURL, session.Done, func(raw string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return s.MicrosoftCopilot.CompleteFromInput(ctx, session.State, raw)
		}, func() {
			s.MicrosoftCopilot.CancelLogin(session.State)
		})
		if err != nil {
			s.MicrosoftCopilot.CancelLogin(session.State)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, job.response())
		return
	}
	cfg = s.attachACPRuntimeConfig(agent, cfg)
	probeCfg := cfg
	probeCfg.Auth = auth
	auth = acp.ProbeAgentAuth(agent, probeCfg, s.runtimeRoot(), nil).RecommendedAuth
	invocation := acp.AgentLoginInvocationFor(agent, s.runtimeRoot(), auth, cfg.LoginBinDir, cfg.Env)
	if !invocation.Available {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%s", invocation.Reason))
		return
	}
	job, err := s.runACPAuthLogin(r.Context(), agent, auth, invocation)
	if err != nil {
		if errors.Is(err, errAntigravityAuthBusy) {
			writeError(w, http.StatusConflict, err)
			return
		}
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
	if err := acp.PrepareAgentLoginInvocation(agent, auth, s.runtimeRoot(), invocation); err != nil {
		return nil, err
	}
	cmdCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acpAuthLoginTimeout)
	cmd := exec.CommandContext(cmdCtx, invocation.Executable, invocation.Args...)
	cmd.Env = acpAuthLoginEnv(invocation)
	cmd.Dir = firstNonEmpty(strings.TrimSpace(invocation.Cwd), strings.TrimSpace(s.runtimeRoot()))
	writer := acpAuthLoginWriter{job: job}
	release, err := s.claimACPAuth(agent)
	if err != nil {
		cancel()
		return nil, err
	}
	outputDone, err := startACPAuthLoginProcess(cmd, job, writer, invocation.UsePTY)
	if err != nil {
		release()
		cancel()
		return nil, err
	}
	s.acpAuthLoginJobs.Store(job.ID, job)
	go func() {
		defer release()
		defer cancel()
		err := cmd.Wait()
		// Let the pty reader deliver the CLI's final lines before closing the
		// master discards them; it ends on its own once the process exits.
		select {
		case <-outputDone:
		case <-time.After(time.Second):
		}
		// The process is done reading; stop accepting input before the (slower)
		// verify so a late paste rejects cleanly instead of writing to a pipe
		// Wait already closed.
		job.closeStdin()
		if err == nil {
			err = s.verifyACPAuthLogin(agent, auth)
		} else if last := job.failureLine(); last != "" {
			// CLI exit codes are opaque; the printed error (e.g. agy's
			// "Error: authentication failed or timed out") is the real story.
			err = errors.New(last)
		}
		release()
		job.finish(err, cmdCtx.Err())
	}()
	return job, nil
}

func (s *Server) startACPOAuthLogin(agent string, auth acp.AgentAuthConfig, authURL string, done <-chan error, onInput func(string) error, onTimeout func()) (*acpAuthLoginJob, error) {
	id, err := newACPAuthLoginID()
	if err != nil {
		return nil, err
	}
	job := &acpAuthLoginJob{
		ID:        id,
		Agent:     agent,
		Status:    "running",
		StartedAt: time.Now().UTC(),
		authURL:   authURL,
		onInput:   onInput,
	}
	s.acpAuthLoginJobs.Store(job.ID, job)
	go func() {
		timer := time.NewTimer(acpAuthLoginTimeout)
		defer timer.Stop()
		select {
		case err := <-done:
			if err == nil {
				err = s.verifyACPAuthLogin(agent, auth)
			}
			job.finish(err, nil)
		case <-timer.C:
			if onTimeout != nil {
				onTimeout()
			}
			job.finish(nil, context.DeadlineExceeded)
		}
	}()
	return job, nil
}

// startACPAuthLoginProcess wires the login process to the job and returns a
// channel that closes once its output has been fully read. Keep stdin open so
// a code the user pastes back can reach a CLI that's blocked waiting for it;
// loopback flows simply never read it. usePTY runs the CLI on a
// pseudo-terminal for CLIs that stall draining piped stdin (agy).
func startACPAuthLoginProcess(cmd *exec.Cmd, job *acpAuthLoginJob, writer acpAuthLoginWriter, usePTY bool) (<-chan struct{}, error) {
	done := make(chan struct{})
	if usePTY {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			return nil, err
		}
		job.stdin = ptmx
		go func() {
			// The copy ends with EIO once the process exits and the pty closes.
			_, _ = io.Copy(writer, ptmx)
			close(done)
		}()
		return done, nil
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	job.stdin = stdin
	// exec.Cmd owns the pipe copies; Wait returns only after output is drained.
	close(done)
	return done, cmd.Start()
}

func (s *Server) verifyACPAuthLogin(agent string, auth acp.AgentAuthConfig) error {
	cfg, _ := s.acpAgentCatalog().Agent(agent)
	cfg.Auth = auth
	cfg = s.attachACPRuntimeConfig(agent, cfg)
	status, readiness := acp.ProbeAuthAndReadinessWithProviders(agent, cfg, s.runtimeRoot(), nil, s.modelProviders())
	if status.Authenticated && readiness.Available {
		s.recordACPAuthProbe(agent, cfg, status, readiness)
		return nil
	}
	s.invalidateACPAuthProbe(agent)
	if status.Authenticated {
		if strings.TrimSpace(readiness.Reason) != "" {
			return fmt.Errorf("%s", readiness.Reason)
		}
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
	processenv.PreserveHost(env,
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"LANG",
		"LC_ALL",
		"LC_CTYPE",
		"LOGNAME",
		"NO_PROXY",
		"SHELL",
		"SSH_AUTH_SOCK",
		"USER",
	)
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

// failureLine picks the login output line that explains a failure: the most
// recent line starting with "Error", else the final non-empty line. Log-tailed
// prompts can land after the CLI's error, so recency alone is not enough.
func (j *acpAuthLoginJob) failureLine() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	clean := acpANSISequence.ReplaceAllString(j.Output, "")
	lines := strings.Split(clean, "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if last == "" {
			last = line
		}
		if strings.HasPrefix(strings.ToLower(line), "error") {
			return line
		}
	}
	return last
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
	if j.Status != "running" {
		j.mu.Unlock()
		return fmt.Errorf("sign-in is not waiting for a code")
	}
	if j.onInput != nil {
		onInput := j.onInput
		j.mu.Unlock()
		return onInput(input)
	}
	if j.stdin == nil {
		j.mu.Unlock()
		return fmt.Errorf("sign-in is not waiting for a code")
	}
	line := strings.TrimRight(input, "\r\n") + "\n"
	_, err := io.WriteString(j.stdin, line)
	j.mu.Unlock()
	if err != nil {
		return fmt.Errorf("could not send the code to %s sign-in: %w", j.Agent, err)
	}
	return nil
}

func (j *acpAuthLoginJob) response() acpAuthLoginResponse {
	j.mu.Lock()
	defer j.mu.Unlock()
	authURL, authCode := acpAuthLoginHints(j.Output)
	if j.authURL != "" {
		authURL = j.authURL
	}
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
		if strings.Contains(match, "auth.") || strings.Contains(match, "claude.com") || strings.Contains(match, "accounts.google.com") {
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
