package cloud

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/processenv"
)

const desktopCloudLoginTimeout = 10 * time.Minute

type DesktopCloudLoginStarter interface {
	StartIsolatedCloudLogin(agent string) (acp.IsolatedCloudLogin, error)
}

type desktopLoginSession struct {
	mu              sync.Mutex
	ID              string    `json:"id"`
	Agent           string    `json:"agent"`
	Status          string    `json:"status"`
	VerificationURL string    `json:"verification_url,omitempty"`
	UserCode        string    `json:"user_code,omitempty"`
	Error           string    `json:"error,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func (s *desktopLoginSession) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"id":               s.ID,
		"agent":            s.Agent,
		"status":           s.Status,
		"verification_url": s.VerificationURL,
		"user_code":        s.UserCode,
		"error":            s.Error,
		"expires_at":       s.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

type ACPProfileHandler struct {
	service *cloudsync.Service
	starter DesktopCloudLoginStarter
	capture func(agent, dir string) (map[string][]byte, error)
	logins  sync.Map
}

func NewACPProfileHandler(service *cloudsync.Service, starter DesktopCloudLoginStarter) *ACPProfileHandler {
	return &ACPProfileHandler{
		service: service,
		starter: starter,
		capture: acp.CaptureIsolatedAuthFiles,
	}
}

func (h *ACPProfileHandler) Proxy(w http.ResponseWriter, r *http.Request) {
	if !h.service.ProxyActiveWorkspace(w, r) {
		WriteError(w, cloudsync.ErrNotConnected)
		return
	}
	if r.Method != http.MethodGet || strings.Contains(r.URL.Path, "/login/") {
		h.service.InvalidateModelsCache()
	}
}

// StartDesktopLogin launches a fresh browser OAuth sign-in in an isolated temp
// directory on Desktop for agents whose provider requires a localhost callback
// (Claude, Kimi), then uploads the brand-new token to the active Cloud workspace
// without touching the user's Local workspace credentials.
func (h *ACPProfileHandler) StartDesktopLogin(w http.ResponseWriter, r *http.Request) {
	if h.starter == nil {
		httpapi.WriteError(w, http.StatusNotImplemented, errors.New("desktop OAuth sign-in is unavailable"))
		return
	}
	if h.service.ActiveWorkspaceID() == cloudsync.LocalWorkspaceID {
		WriteError(w, cloudsync.ErrNotConnected)
		return
	}
	agent := strings.ToLower(strings.TrimSpace(r.PathValue("agent")))
	iso, err := h.starter.StartIsolatedCloudLogin(agent)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	id := newDesktopLoginID()
	sess := &desktopLoginSession{
		ID:        id,
		Agent:     iso.Agent,
		Status:    "pending",
		ExpiresAt: time.Now().UTC().Add(desktopCloudLoginTimeout),
	}
	h.logins.Store(id, sess)

	ctx, cancel := context.WithTimeout(context.Background(), desktopCloudLoginTimeout)
	cmd := exec.CommandContext(ctx, iso.Invocation.Executable, iso.Invocation.Args...)
	cmd.Dir = iso.Dir
	cmd.Env = isolatedLoginEnv(iso.Invocation)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		cancel()
		_ = pw.Close()
		_ = pr.Close()
		iso.Cleanup()
		h.logins.Delete(id)
		httpapi.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	go h.watchDesktopLogin(ctx, cancel, cmd, pr, pw, iso, sess)
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]any{"session": sess.snapshot()})
}

func (h *ACPProfileHandler) GetDesktopLogin(w http.ResponseWriter, r *http.Request) {
	agent := strings.ToLower(strings.TrimSpace(r.PathValue("agent")))
	id := strings.TrimSpace(r.PathValue("id"))
	raw, ok := h.logins.Load(id)
	if !ok {
		httpapi.WriteError(w, http.StatusNotFound, errors.New("login session not found"))
		return
	}
	sess := raw.(*desktopLoginSession)
	if sess.Agent != agent {
		httpapi.WriteError(w, http.StatusNotFound, errors.New("login session not found"))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"session": sess.snapshot()})
}

func (h *ACPProfileHandler) watchDesktopLogin(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	pr *io.PipeReader,
	pw *io.PipeWriter,
	iso acp.IsolatedCloudLogin,
	sess *desktopLoginSession,
) {
	defer cancel()
	defer iso.Cleanup()
	defer func() {
		time.AfterFunc(5*time.Minute, func() { h.logins.Delete(sess.ID) })
	}()

	var tail strings.Builder
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 4096), 64*1024)
		for scanner.Scan() {
			line := desktopANSIRe.ReplaceAllString(scanner.Text(), "")
			if tail.Len() < 4096 {
				tail.WriteString(line)
				tail.WriteByte('\n')
			}
			u := strings.TrimRight(desktopURLRe.FindString(line), ".,);:")
			c := desktopCodeRe.FindString(line)
			if u != "" || c != "" {
				sess.mu.Lock()
				if u != "" && sess.VerificationURL == "" {
					sess.VerificationURL = u
				}
				if c != "" && sess.UserCode == "" {
					sess.UserCode = c
				}
				if sess.Status == "pending" {
					sess.Status = "awaiting_user"
				}
				sess.mu.Unlock()
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
		sess.mu.Lock()
		if ctx.Err() == context.DeadlineExceeded {
			sess.Status = "expired"
			sess.Error = "sign-in timed out"
		} else {
			sess.Status = "failed"
			sess.Error = msg
		}
		sess.mu.Unlock()
		return
	}

	files, err := h.capture(iso.Agent, iso.Dir)
	if err != nil {
		sess.mu.Lock()
		sess.Status = "failed"
		sess.Error = err.Error()
		sess.mu.Unlock()
		return
	}
	encoded := make(map[string]string, len(files))
	for rel, raw := range files {
		encoded[rel] = base64.StdEncoding.EncodeToString(raw)
	}
	payload, err := json.Marshal(map[string]any{"files": encoded})
	if err != nil {
		sess.mu.Lock()
		sess.Status = "failed"
		sess.Error = err.Error()
		sess.mu.Unlock()
		return
	}
	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer uploadCancel()
	_, status, err := h.service.PutACPProfile(uploadCtx, iso.Agent, payload)
	if err != nil || status >= 300 {
		msg := "failed to save credentials to Jaz Cloud"
		if err != nil {
			msg = err.Error()
		}
		sess.mu.Lock()
		sess.Status = "failed"
		sess.Error = msg
		sess.mu.Unlock()
		return
	}

	sess.mu.Lock()
	sess.Status = "completed"
	sess.Error = ""
	sess.mu.Unlock()
}

func isolatedLoginEnv(inv acp.AgentLoginInvocation) []string {
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

func newDesktopLoginID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "dlog_" + hex.EncodeToString(b[:])
}

var (
	desktopANSIRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	desktopURLRe  = regexp.MustCompile(`https://[^\s"'<>]+`)
	desktopCodeRe = regexp.MustCompile(`\b[A-Z0-9]{4,5}-[A-Z0-9]{4,5}\b`)
)
