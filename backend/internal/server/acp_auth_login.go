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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
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
}

func (s *Server) handleStartACPAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	agent := acp.CanonicalAgentName(r.PathValue("agent"))
	if _, ok := s.acpAgentCatalog().Agent(agent); !ok {
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
	auth, err := acp.NormalizeAgentAuthConfig(agent, input.Auth)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	auth = acp.ProbeAgentAuth(agent, acp.AgentConfig{Auth: auth}, s.runtimeRoot(), nil).RecommendedAuth
	invocation := acp.AgentLoginInvocationFor(agent, s.runtimeRoot(), auth)
	if !invocation.Available {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%s", invocation.Reason))
		return
	}
	job, err := s.runACPAuthLogin(r.Context(), agent, invocation)
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

func (s *Server) runACPAuthLogin(ctx context.Context, agent string, invocation acp.AgentLoginInvocation) (*acpAuthLoginJob, error) {
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
	// Login writes credentials into the agent's profile dir (CODEX_HOME /
	// CLAUDE_CONFIG_DIR). The CLI won't create it — codex aborts with "CODEX_HOME
	// points to … but that path does not exist" — so make it first. These are
	// the explicit Jaz-owned profile paths, the only dirs Jaz creates for agents.
	if err := ensureLoginProfileDirs(invocation.Env); err != nil {
		return nil, err
	}
	cmdCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acpAuthLoginTimeout)
	cmd := exec.CommandContext(cmdCtx, invocation.Executable, invocation.Args...)
	cmd.Env = acpAuthLoginEnv(invocation)
	if root := strings.TrimSpace(s.runtimeRoot()); root != "" {
		cmd.Dir = root
	}
	writer := acpAuthLoginWriter{job: job}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	s.acpAuthLoginJobs.Store(job.ID, job)
	go func() {
		defer cancel()
		err := cmd.Wait()
		job.finish(err, cmdCtx.Err())
	}()
	return job, nil
}

// ensureLoginProfileDirs creates the profile directories a login invocation
// points its CLI at. The invocation env only carries explicit profile paths
// (CODEX_HOME, CLAUDE_CONFIG_DIR); Grok carries none and uses the real home.
func ensureLoginProfileDirs(env map[string]string) error {
	for key, dir := range env {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("prepare %s profile %s: %w", key, dir, err)
		}
	}
	return nil
}

func newACPAuthLoginID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "login_" + hex.EncodeToString(b[:]), nil
}

func acpAuthLoginEnv(invocation acp.AgentLoginInvocation) []string {
	env := map[string]string{}
	for _, key := range []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "LOGNAME", "SHELL", "SSH_AUTH_SOCK", "USER"} {
		if value := os.Getenv(key); strings.TrimSpace(value) != "" {
			env[key] = value
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
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
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
