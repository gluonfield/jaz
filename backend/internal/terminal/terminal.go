package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/creack/pty"
	"github.com/wins/jaz/backend/internal/shellcmd"
)

const defaultReplayBytes = 128 * 1024

var ErrUnsupported = errors.New("terminal sessions are not supported on Windows yet")

type Size struct {
	Cols uint16
	Rows uint16
}

type Message struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cwd  string `json:"cwd,omitempty"`
	Code int    `json:"code,omitempty"`
	Err  string `json:"error,omitempty"`
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	replay   int
}

func New() *Manager {
	return &Manager{sessions: map[string]*Session{}, replay: defaultReplayBytes}
}

func (m *Manager) Open(id, cwd string, size Size) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if cwd == "" {
		return nil, fmt.Errorf("working directory is required")
	}
	cwd = filepath.Clean(cwd)
	if info, err := os.Stat(cwd); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("working directory is not a directory: %s", cwd)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if session := m.sessions[id]; session != nil && session.Alive() {
		if session.Cwd() == cwd {
			return session, nil
		}
		delete(m.sessions, id)
		session.Terminate()
	}
	session := newSession(cwd, m.replay, func(s *Session) {
		m.mu.Lock()
		if m.sessions[id] == s {
			delete(m.sessions, id)
		}
		m.mu.Unlock()
	})
	if err := session.start(size); err != nil {
		return nil, err
	}
	m.sessions[id] = session
	return session, nil
}

func (m *Manager) Kill(id string) {
	m.mu.Lock()
	session := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if session != nil {
		session.Terminate()
	}
}

func (m *Manager) Active(id string) bool {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	return session != nil && session.Alive()
}

func (m *Manager) Close() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = map[string]*Session{}
	m.mu.Unlock()
	for _, session := range sessions {
		session.Terminate()
	}
}

type Session struct {
	cwd       string
	replayMax int
	onExit    func(*Session)

	mu      sync.Mutex
	cmd     *exec.Cmd
	file    *os.File
	closed  bool
	buffer  []byte
	clients map[chan Message]struct{}
}

func newSession(cwd string, replayMax int, onExit func(*Session)) *Session {
	return &Session{
		cwd:       cwd,
		replayMax: replayMax,
		clients:   map[chan Message]struct{}{},
		onExit:    onExit,
	}
}

func (s *Session) Cwd() string {
	return s.cwd
}

func (s *Session) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *Session) Replay() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(append([]byte(nil), s.buffer...))
}

func (s *Session) Subscribe() (<-chan Message, func()) {
	ch := make(chan Message, 128)
	s.mu.Lock()
	if s.closed {
		close(ch)
		s.mu.Unlock()
		return ch, func() {}
	}
	s.clients[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.clients[ch]; ok {
			delete(s.clients, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}

func (s *Session) Write(data string) error {
	s.mu.Lock()
	file := s.file
	closed := s.closed
	s.mu.Unlock()
	if closed || file == nil {
		return io.ErrClosedPipe
	}
	_, err := io.WriteString(file, data)
	return err
}

func (s *Session) Resize(size Size) error {
	if size.Cols == 0 || size.Rows == 0 {
		return nil
	}
	s.mu.Lock()
	file := s.file
	closed := s.closed
	s.mu.Unlock()
	if closed || file == nil {
		return io.ErrClosedPipe
	}
	return pty.Setsize(file, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
}

func (s *Session) Terminate() {
	s.mu.Lock()
	file := s.file
	cmd := s.cmd
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	if file != nil {
		_ = file.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (s *Session) start(size Size) error {
	if runtime.GOOS == "windows" {
		return ErrUnsupported
	}
	cmd := exec.Command(shellcmd.DefaultShell())
	cmd.Dir = s.cwd
	cmd.Env = terminalEnv(os.Environ())
	win := &pty.Winsize{Cols: firstNonZero(size.Cols, 80), Rows: firstNonZero(size.Rows, 24)}
	file, err := pty.StartWithSize(cmd, win)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cmd = cmd
	s.file = file
	s.mu.Unlock()
	go s.read()
	go s.wait()
	return nil
}

func (s *Session) read() {
	buf := make([]byte, 4096)
	for {
		n, err := s.file.Read(buf)
		if n > 0 {
			s.output(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	code := exitCode(err)
	msg := Message{Type: "exit", Code: code}
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		msg.Err = err.Error()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	clients := s.clients
	s.clients = map[chan Message]struct{}{}
	for ch := range clients {
		select {
		case ch <- msg:
		default:
		}
		close(ch)
	}
	s.mu.Unlock()
	_ = s.file.Close()
	if s.onExit != nil {
		s.onExit(s)
	}
}

func (s *Session) output(data []byte) {
	text := string(data)
	s.mu.Lock()
	s.buffer = append(s.buffer, data...)
	if extra := len(s.buffer) - s.replayMax; extra > 0 {
		s.buffer = append([]byte(nil), s.buffer[extra:]...)
	}
	msg := Message{Type: "output", Data: text}
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
	s.mu.Unlock()
}

func terminalEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	seenTerm := false
	seenColor := false
	for _, value := range env {
		if len(value) >= 5 && value[:5] == "TERM=" {
			seenTerm = true
			out = append(out, "TERM=xterm-256color")
			continue
		}
		if len(value) >= 10 && value[:10] == "COLORTERM=" {
			seenColor = true
			out = append(out, "COLORTERM=truecolor")
			continue
		}
		out = append(out, value)
	}
	if !seenTerm {
		out = append(out, "TERM=xterm-256color")
	}
	if !seenColor {
		out = append(out, "COLORTERM=truecolor")
	}
	return out
}

func firstNonZero(value, fallback uint16) uint16 {
	if value != 0 {
		return value
	}
	return fallback
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
