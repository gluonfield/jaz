package machinebridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultCodexBinary     = "codex"
	defaultExecStartWindow = 10 * time.Second
	execStopGracePeriod    = 2 * time.Second
	maxExecStderrCapture   = 8192
	maxExecLineInspect     = 4096
)

type CodexExecConfig struct {
	Guard          *PolicyGuard
	Binary         string
	Args           []string
	StartupTimeout time.Duration
}

type codexExecSession struct {
	id       string
	endpoint string
	cmd      *exec.Cmd
	done     chan struct{}
	waitErr  error
}

// CodexExecLauncher spawns a native `codex exec-server` subprocess scoped to an
// approved writable folder grant with per-session capability-token WS auth.
type CodexExecLauncher struct {
	guard          *PolicyGuard
	binary         string
	args           []string
	startupTimeout time.Duration

	baseCtx context.Context
	cancel  context.CancelFunc

	mu       sync.Mutex
	closed   bool
	starting sync.WaitGroup
	sessions map[string]*codexExecSession
}

func NewCodexExecLauncher(cfg CodexExecConfig) (*CodexExecLauncher, error) {
	if cfg.Guard == nil {
		return nil, fmt.Errorf("%w: policy guard is required", ErrInvalidRequest)
	}
	binary := strings.TrimSpace(cfg.Binary)
	if binary == "" {
		binary = defaultCodexBinary
	}
	var args []string
	if len(cfg.Args) > 0 {
		args = append([]string(nil), cfg.Args...)
	}
	timeout := cfg.StartupTimeout
	if timeout <= 0 {
		timeout = defaultExecStartWindow
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	return &CodexExecLauncher{
		guard:          cfg.Guard,
		binary:         binary,
		args:           args,
		startupTimeout: timeout,
		baseCtx:        baseCtx,
		cancel:         cancel,
		sessions:       make(map[string]*codexExecSession),
	}, nil
}

func (l *CodexExecLauncher) Start(ctx context.Context, req ExecStartRequest) (ExecStartResponse, error) {
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" {
		return ExecStartResponse{}, fmt.Errorf("%w: grant_id and run_id are required", ErrInvalidRequest)
	}
	if len(req.ReadOnlyDirs) > 0 {
		return ExecStartResponse{}, fmt.Errorf("%w: read_only_dirs is not supported by codex exec-server", ErrUnsupportedCapability)
	}
	grantRoot, err := l.guard.GrantRoot(req.GrantID, OpWrite)
	if err != nil {
		return ExecStartResponse{}, err
	}

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return ExecStartResponse{}, errors.New("exec launcher is closed")
	}
	l.starting.Add(1)
	l.mu.Unlock()
	defer l.starting.Done()

	binaryPath, err := exec.LookPath(l.binary)
	if err != nil {
		return ExecStartResponse{}, fmt.Errorf("locate %s: %w", l.binary, err)
	}
	sessionID, err := newExecSessionID()
	if err != nil {
		return ExecStartResponse{}, err
	}
	authToken, tokenSHA256, err := newExecAuthToken()
	if err != nil {
		return ExecStartResponse{}, err
	}

	args := l.args
	if len(args) == 0 {
		// Standalone `codex exec-server` rejects --exit-on-stdin-close without --remote
		// and --environment-id, so lifecycle cleanup is enforced via process groups,
		// disconnect StopAll(), and launcher Close().
		args = []string{
			"exec-server",
			"--listen=ws://127.0.0.1:0",
			"--ws-auth=capability-token",
			"--ws-token-sha256=" + tokenSHA256,
		}
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = grantRoot
	cmd.Env = filteredExecEnv(os.Environ())
	configureExecProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ExecStartResponse{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ExecStartResponse{}, err
	}
	if err := cmd.Start(); err != nil {
		return ExecStartResponse{}, fmt.Errorf("start %s: %w", l.binary, err)
	}

	sess := &codexExecSession{
		id:   sessionID,
		cmd:  cmd,
		done: make(chan struct{}),
	}

	var stderrBuf cappedBuffer
	stderrBuf.limit = maxExecStderrCapture
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	endpointCh := make(chan string, 1)
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		if endpoint, ok := scanForLoopbackWSEndpoint(stdout); ok {
			select {
			case endpointCh <- endpoint:
			default:
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()

	go func() {
		<-stdoutDone
		<-stderrDone
		sess.waitErr = cmd.Wait()
		l.mu.Lock()
		delete(l.sessions, sessionID)
		l.mu.Unlock()
		close(sess.done)
	}()

	startCtx, cancel := context.WithTimeout(ctx, l.startupTimeout)
	defer cancel()
	stopCancel := context.AfterFunc(l.baseCtx, cancel)
	defer stopCancel()

	var endpoint string
	select {
	case endpoint = <-endpointCh:
	case <-sess.done:
		msg := strings.TrimSpace(stderrBuf.String())
		if msg != "" {
			return ExecStartResponse{}, fmt.Errorf("codex exec-server exited before listening: %s", msg)
		}
		return ExecStartResponse{}, fmt.Errorf("codex exec-server exited before listening: %v", sess.waitErr)
	case <-startCtx.Done():
		stopExecSession(context.Background(), sess)
		return ExecStartResponse{}, fmt.Errorf("timed out waiting for codex exec-server endpoint: %w", startCtx.Err())
	}

	if err := waitEndpointReady(startCtx, endpoint, sess.done); err != nil {
		stopExecSession(context.Background(), sess)
		return ExecStartResponse{}, err
	}
	sess.endpoint = endpoint

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		stopExecSession(context.Background(), sess)
		return ExecStartResponse{}, errors.New("exec launcher is closed")
	}
	select {
	case <-sess.done:
		l.mu.Unlock()
		return ExecStartResponse{}, errors.New("codex exec-server exited unexpectedly")
	default:
		l.sessions[sessionID] = sess
	}
	l.mu.Unlock()

	return ExecStartResponse{
		SessionID: sessionID,
		Endpoint:  endpoint,
		AuthToken: authToken,
	}, nil
}

func (l *CodexExecLauncher) Stop(ctx context.Context, req ExecStopRequest) error {
	id := strings.TrimSpace(req.SessionID)
	if id == "" {
		return fmt.Errorf("%w: session_id is required", ErrInvalidRequest)
	}
	l.mu.Lock()
	sess, ok := l.sessions[id]
	if ok {
		delete(l.sessions, id)
	}
	l.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: unknown exec session %q", ErrNotFound, id)
	}
	stopExecSession(ctx, sess)
	return nil
}

func (l *CodexExecLauncher) StopAll() {
	l.mu.Lock()
	active := make([]*codexExecSession, 0, len(l.sessions))
	for id, sess := range l.sessions {
		active = append(active, sess)
		delete(l.sessions, id)
	}
	l.mu.Unlock()

	var wg sync.WaitGroup
	for _, sess := range active {
		wg.Add(1)
		go func(s *codexExecSession) {
			defer wg.Done()
			stopExecSession(context.Background(), s)
		}(sess)
	}
	wg.Wait()
}

func (l *CodexExecLauncher) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()

	l.cancel()
	l.starting.Wait()
	l.StopAll()
	return nil
}

func (l *CodexExecLauncher) ActiveCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.sessions)
}

func stopExecSession(ctx context.Context, sess *codexExecSession) {
	select {
	case <-sess.done:
		return
	default:
	}
	terminateExecProcessGroup(sess.cmd)
	timer := time.NewTimer(execStopGracePeriod)
	defer timer.Stop()
	select {
	case <-sess.done:
		return
	case <-timer.C:
	case <-ctx.Done():
	}
	killExecProcessGroup(sess.cmd)
	<-sess.done
}

func filteredExecEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if isAllowedExecEnvKey(key) {
			out = append(out, entry)
		}
	}
	return out
}

func isAllowedExecEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	switch upper {
	case "PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "LANG",
		"TMPDIR", "TEMP", "TMP", "SSL_CERT_FILE", "SSL_CERT_DIR",
		"SYSTEMROOT", "COMSPEC", "PATHEXT", "WINDIR":
		return true
	}
	return strings.HasPrefix(upper, "LC_") || strings.HasPrefix(upper, "XDG_")
}

// scanForLoopbackWSEndpoint reads lines of arbitrary length without failing on >64 KiB output.
func scanForLoopbackWSEndpoint(r io.Reader) (string, bool) {
	br := bufio.NewReaderSize(r, 64*1024)
	var lineBuf bytes.Buffer
	for {
		chunk, isPrefix, err := br.ReadLine()
		if len(chunk) > 0 && lineBuf.Len() < maxExecLineInspect {
			rem := maxExecLineInspect - lineBuf.Len()
			if len(chunk) > rem {
				_, _ = lineBuf.Write(chunk[:rem])
			} else {
				_, _ = lineBuf.Write(chunk)
			}
		}
		if !isPrefix {
			if endpoint, ok := parseLoopbackWSEndpoint(lineBuf.String()); ok {
				return endpoint, true
			}
			lineBuf.Reset()
		}
		if err != nil {
			if lineBuf.Len() > 0 {
				if endpoint, ok := parseLoopbackWSEndpoint(lineBuf.String()); ok {
					return endpoint, true
				}
			}
			return "", false
		}
	}
}

func parseLoopbackWSEndpoint(line string) (string, bool) {
	for field := range strings.FieldsSeq(strings.TrimSpace(line)) {
		trimmed := strings.Trim(field, `"'(),.;`)
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Scheme != "ws" || parsed.Host == "" {
			continue
		}
		host, port, err := net.SplitHostPort(parsed.Host)
		if err != nil || port == "" || port == "0" || !isLoopbackHost(host) {
			continue
		}
		return parsed.String(), true
	}
	return "", false
}

func waitEndpointReady(ctx context.Context, endpoint string, done <-chan struct{}) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	for {
		conn, err := dialer.DialContext(ctx, "tcp", parsed.Host)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-done:
			return errors.New("codex exec-server exited before accepting connections")
		case <-ctx.Done():
			return fmt.Errorf("codex exec-server %s not reachable: %w", endpoint, ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func newExecSessionID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "exec-" + hex.EncodeToString(raw[:]), nil
}

func newExecAuthToken() (token string, sha256Hex string, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

type cappedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if rem := b.limit - b.buf.Len(); rem > 0 {
		if len(p) > rem {
			_, _ = b.buf.Write(p[:rem])
		} else {
			_, _ = b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
