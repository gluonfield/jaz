package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/shellcmd"
	"github.com/wins/jaz/backend/internal/tools"
)

type CommandManager struct {
	nextID   atomic.Int64
	mu       sync.Mutex
	sessions map[int64]*commandSession
}

func NewCommandManager() *CommandManager {
	return &CommandManager{sessions: make(map[int64]*commandSession)}
}

type commandSession struct {
	id      int64
	cmd     *osexec.Cmd
	stdin   io.WriteCloser
	started time.Time

	mu       sync.Mutex
	output   bytes.Buffer
	readPos  int
	done     chan error
	waitErr  error
	finished bool
}

func (s *commandSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.Write(p)
}

func (s *commandSession) readRecent(maxChars int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := s.output.String()
	if s.readPos > len(data) {
		s.readPos = len(data)
	}
	recent := data[s.readPos:]
	s.readPos = len(data)
	if maxChars > 0 && len(recent) > maxChars {
		return recent[len(recent)-maxChars:]
	}
	return recent
}

func (s *commandSession) markDone(err error) {
	s.mu.Lock()
	s.waitErr = err
	s.finished = true
	s.mu.Unlock()
	close(s.done)
}

func (s *commandSession) snapshotDone() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished, s.waitErr
}

type ExecCommandTool struct {
	Manager   *CommandManager
	Workspace string
}

func (t *ExecCommandTool) Definition() tools.Definition {
	return tools.Function(
		"exec_command",
		"Runs a local shell command. Do not use when the user explicitly asks for a separate Jaz agent such as codex, claude, or grok; use jazagent_spawn instead.",
		false,
		tools.ObjectSchema(map[string]any{
			"cmd":               tools.StringSchema("Shell command to execute."),
			"workdir":           tools.StringSchema("Working directory for the command. Defaults to the turn cwd."),
			"shell":             tools.StringSchema("Shell binary to launch. Defaults to the user's default shell."),
			"tty":               tools.BoolSchema("True allocates a PTY for the command; false or omitted uses plain pipes."),
			"yield_time_ms":     tools.NumberSchema("Wait before yielding output. Defaults to 10000 ms; effective range is 250-30000 ms."),
			"max_output_tokens": tools.NumberSchema("Output token budget. Defaults to 10000 tokens; larger requests may be capped by policy."),
			"login":             tools.BoolSchema("True runs the shell with -l/-i semantics; false disables them. Defaults to true."),
		}, []string{"cmd"}),
	)
}

func (t *ExecCommandTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Manager == nil {
		return tools.Result{}, errors.New("exec_command manager is nil")
	}
	cmdText := tools.StringInput(inputs, "cmd")
	if cmdText == "" {
		return tools.Result{}, errors.New("cmd is required")
	}
	if tools.BoolInput(inputs, "tty", false) {
		return tools.Result{}, errors.New("tty=true is not implemented in this MVP")
	}

	yield := tools.Clamp(tools.IntInput(inputs, "yield_time_ms", 10000), 250, 30000)
	maxTokens := tools.Clamp(tools.IntInput(inputs, "max_output_tokens", 10000), 1, 50000)
	maxChars := maxTokens * 4
	base, err := sessioncontext.SessionBase(ctx, t.Workspace)
	if err != nil {
		return tools.Result{}, err
	}
	workdir, err := resolveWorkdir(base, tools.StringInput(inputs, "workdir"))
	if err != nil {
		return tools.Result{}, err
	}

	shell := tools.StringInput(inputs, "shell")
	if shell == "" {
		shell = shellcmd.DefaultShell()
	}
	login := tools.BoolInput(inputs, "login", true)

	payload, err := t.Manager.exec(ctx, commandRequest{
		Command:  cmdText,
		Workdir:  workdir,
		Shell:    shell,
		Login:    login,
		Yield:    time.Duration(yield) * time.Millisecond,
		MaxChars: maxChars,
	})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(payload)
}

type WriteStdinTool struct {
	Manager *CommandManager
}

func (t *WriteStdinTool) Definition() tools.Definition {
	return tools.Function(
		"write_stdin",
		"Writes characters to an existing unified exec session and returns recent output.",
		false,
		tools.ObjectSchema(map[string]any{
			"session_id":        tools.NumberSchema("Identifier of the running unified exec session."),
			"chars":             tools.StringSchema("Bytes to write to stdin. Defaults to empty, which polls without writing."),
			"yield_time_ms":     tools.NumberSchema("Wait before yielding output. Non-empty writes default to 250 ms and cap at 30000 ms; empty polls wait 5000-300000 ms by default."),
			"max_output_tokens": tools.NumberSchema("Output token budget. Defaults to 10000 tokens; larger requests may be capped by policy."),
		}, []string{"session_id"}),
	)
}

func (t *WriteStdinTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if t.Manager == nil {
		return tools.Result{}, errors.New("write_stdin manager is nil")
	}
	sessionID := int64(tools.IntInput(inputs, "session_id", 0))
	if sessionID == 0 {
		return tools.Result{}, errors.New("session_id is required")
	}
	chars := tools.StringInput(inputs, "chars")
	defaultYield := 5000
	if chars != "" {
		defaultYield = 250
	}
	yield := tools.Clamp(tools.IntInput(inputs, "yield_time_ms", defaultYield), 0, 300000)
	maxTokens := tools.Clamp(tools.IntInput(inputs, "max_output_tokens", 10000), 1, 50000)
	payload, err := t.Manager.write(ctx, sessionID, chars, time.Duration(yield)*time.Millisecond, maxTokens*4)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(payload)
}

type commandRequest struct {
	Command  string
	Workdir  string
	Shell    string
	Login    bool
	Yield    time.Duration
	MaxChars int
}

func (m *CommandManager) exec(ctx context.Context, req commandRequest) (map[string]any, error) {
	shell, args := shellcmd.Command(req.Shell, req.Command, req.Login)
	cmd := osexec.Command(shell, args...)
	cmd.Dir = req.Workdir
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	session := &commandSession{
		id:      m.nextID.Add(1),
		cmd:     cmd,
		stdin:   stdin,
		started: time.Now(),
		done:    make(chan error),
	}
	cmd.Stdout = session
	cmd.Stderr = session
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() { session.markDone(cmd.Wait()) }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	case err := <-session.done:
		output := session.readRecent(req.MaxChars)
		return completedPayload(output, err), nil
	case <-time.After(req.Yield):
		m.mu.Lock()
		m.sessions[session.id] = session
		m.mu.Unlock()
		return map[string]any{
			"status":     "running",
			"session_id": session.id,
			"output":     session.readRecent(req.MaxChars),
		}, nil
	}
}

func (m *CommandManager) write(ctx context.Context, id int64, chars string, yield time.Duration, maxChars int) (map[string]any, error) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("session %d not found", id)
	}

	if chars != "" {
		if _, err := io.WriteString(session.stdin, chars); err != nil {
			return nil, err
		}
	}

	timer := time.NewTimer(yield)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-session.done:
	case <-timer.C:
	}

	finished, waitErr := session.snapshotDone()
	output := session.readRecent(maxChars)
	if finished {
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
		return completedPayload(output, waitErr), nil
	}
	return map[string]any{
		"status":     "running",
		"session_id": id,
		"output":     output,
	}, nil
}

func completedPayload(output string, err error) map[string]any {
	payload := map[string]any{
		"status": "completed",
		"output": output,
	}
	if err == nil {
		payload["exit_code"] = 0
		return payload
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		payload["exit_code"] = exitErr.ExitCode()
	} else {
		payload["exit_code"] = -1
	}
	payload["error"] = err.Error()
	return payload
}

func resolveWorkdir(base, workdir string) (string, error) {
	if base == "" {
		base = "."
	}
	if workdir == "" {
		workdir = base
	} else if !filepath.IsAbs(workdir) {
		workdir = filepath.Join(base, workdir)
	}
	cleaned, err := filepath.Abs(workdir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return "", err
	}
	return cleaned, nil
}
