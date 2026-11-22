package machinebridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultCodexBinary     = "codex"
	defaultExecStartWindow = 10 * time.Second
	execStopGracePeriod    = 2 * time.Second
	maxExecStderrCapture   = 8192
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
// approved folder grant, matching OpenAI's local exec-server model.
type CodexExecLauncher struct {
	guard          *PolicyGuard
	binary         string
	args           []string
	startupTimeout time.Duration

	mu       sync.Mutex
	closed   bool
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
	args := cfg.Args
	if len(args) == 0 {
		args = []string{"exec-server", "--listen=ws://127.0.0.1:0"}
	} else {
		args = append([]string(nil), args...)
	}
	timeout := cfg.StartupTimeout
	if timeout <= 0 {
		timeout = defaultExecStartWindow
	}
	return &CodexExecLauncher{
		guard:          cfg.Guard,
		binary:         binary,
		args:           args,
		startupTimeout: timeout,
		sessions:       make(map[string]*codexExecSession),
	}, nil
}

func (l *CodexExecLauncher) Start(ctx context.Context, req ExecStartRequest, _ string) (ExecStartResponse, error) {
	if strings.TrimSpace(req.GrantID) == "" || strings.TrimSpace(req.RunID) == "" {
		return ExecStartResponse{}, fmt.Errorf("%w: grant_id and run_id are required", ErrInvalidRequest)
	}
	root, err := l.guard.Resolve(req.GrantID, ".", OpRead)
	if err != nil {
		return ExecStartResponse{}, err
	}
	for _, roDir := range req.ReadOnlyDirs {
		if _, err := l.guard.Resolve(req.GrantID, roDir, OpRead); err != nil {
			return ExecStartResponse{}, err
		}
	}

	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return ExecStartResponse{}, errors.New("exec launcher is closed")
	}

	binaryPath, err := exec.LookPath(l.binary)
	if err != nil {
		return ExecStartResponse{}, fmt.Errorf("locate %s: %w", l.binary, err)
	}

	cmd := exec.Command(binaryPath, l.args...)
	cmd.Dir = root.RealPath
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

	sessionID, err := newExecSessionID()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return ExecStartResponse{}, err
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
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if endpoint, ok := parseLoopbackWSEndpoint(scanner.Text()); ok {
				select {
				case endpointCh <- endpoint:
				default:
				}
				break
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

func (l *CodexExecLauncher) Close() error {
	l.mu.Lock()
	l.closed = true
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
	if sess.cmd.Process != nil {
		if runtime.GOOS == "windows" {
			_ = sess.cmd.Process.Kill()
		} else {
			_ = sess.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	timer := time.NewTimer(execStopGracePeriod)
	defer timer.Stop()
	select {
	case <-sess.done:
		return
	case <-timer.C:
	case <-ctx.Done():
	}
	if sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
	}
	<-sess.done
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
