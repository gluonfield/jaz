package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/wins/jaz/backend/internal/processenv"
)

const (
	antigravityWizardTick = 400 * time.Millisecond
	// Every wizard screen but the browser hand-off answers at once, so one that
	// lasts this long is a screen the steps do not cover.
	antigravityWizardStall = 90 * time.Second
)

type antigravityWizardStep struct {
	prompt         string
	answer         func(screen string) string
	waitsOnBrowser bool
}

// agy offers pay-as-you-go billing only in its interactive sign-in wizard,
// which asks these in order.
func antigravityPayAsYouGoSteps(login AntigravityCloudLogin) []antigravityWizardStep {
	return []antigravityWizardStep{
		{prompt: "Select login method", answer: antigravityMenuChoice("Use a Google Cloud project")},
		{prompt: "Google Cloud sign-in method", answer: antigravityMenuChoice("Continue with Google Cloud")},
		{prompt: "Select billing model", answer: antigravityMenuChoice("Pay As You Go"), waitsOnBrowser: true},
		{prompt: "Provide Google Cloud project", answer: antigravityTypedLine(login.Project)},
		// The location prompt's wording is not known, only that it names one.
		{prompt: "ocation", answer: antigravityTypedLine(login.Location)},
	}
}

var antigravityMenuItem = regexp.MustCompile(`(?m)^\s*(>?)\s*(\d+)\. (.*)$`)

func antigravityMenuChoice(label string) func(string) string {
	return func(screen string) string {
		cursor, target := 0, 0
		for _, item := range antigravityMenuItem.FindAllStringSubmatch(screen, -1) {
			n, _ := strconv.Atoi(item[2])
			if item[1] != "" {
				cursor = n
			}
			if strings.Contains(item[3], label) {
				target = n
			}
		}
		if cursor == 0 || target == 0 {
			return ""
		}
		if target < cursor {
			return strings.Repeat("\x1b[A", cursor-target) + "\r"
		}
		return strings.Repeat("\x1b[B", target-cursor) + "\r"
	}
}

// Ctrl+U first, so a value the wizard pre-filled is replaced, not extended.
func antigravityTypedLine(value string) func(string) string {
	return func(string) string { return "\x15" + value + "\r" }
}

var (
	antigravityTerminalControl = regexp.MustCompile(`\x1b\][^\x07]*\x07|\x1b\[[ -?]*[@-~]`)
	// agy wraps the sign-in URL across screen lines, but the hyperlink it draws
	// beside it carries the URL whole.
	antigravitySignInLink = regexp.MustCompile(`\x1b\]8;[^;]*;(https://[^\x07]+)\x07`)
)

type antigravityScreen struct {
	mu  sync.Mutex
	raw []byte
}

func (s *antigravityScreen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append(s.raw, p...)
	return len(p), nil
}

func (s *antigravityScreen) take() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := string(s.raw)
	s.raw = s.raw[:0]
	return raw
}

// runAntigravityWizard runs agy's sign-in wizard on a pseudo-terminal nobody
// sees and answers each step as its prompt appears. agy only redraws the cells
// that changed, so every tick resizes the terminal to make it repaint the whole
// screen for the next read.
func runAntigravityWizard(ctx context.Context, cfg AgentConfig, root string, steps []antigravityWizardStep, browser AntigravityBrowser) error {
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	env := manager.probeEnv(AgentAntigravity, cfg)
	executable, err := antigravityExecutable(env["PATH"])
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable)
	cmd.Env = append(processenv.List(env), "TERM=xterm-256color")
	cmd.Dir = antigravityLoginWorkspace(root)
	if err := os.MkdirAll(cmd.Dir, 0o700); err != nil {
		return err
	}
	size := pty.Winsize{Rows: 40, Cols: 120}
	tty, err := pty.StartWithSize(cmd, &size)
	if errors.Is(err, pty.ErrUnsupported) {
		return fmt.Errorf("pay-as-you-go sign-in is not available on %s yet", runtime.GOOS)
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = tty.Close()
		_ = cmd.Wait()
	}()
	var screen antigravityScreen
	go func() { _, _ = io.Copy(&screen, tty) }()

	ticker := time.NewTicker(antigravityWizardTick)
	defer ticker.Stop()
	step, answered, shown, page := 0, time.Now(), "", ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case code := <-browser.Codes:
			if _, err := io.WriteString(tty, code+"\r"); err != nil {
				return fmt.Errorf("answer Antigravity sign-in: %w", err)
			}
			continue
		case <-ticker.C:
		}
		raw := screen.take()
		if link := antigravitySignInLink.FindStringSubmatch(raw); link != nil && link[1] != page {
			page = link[1]
			browser.Page(page)
		}
		frame := antigravityTerminalControl.ReplaceAllString(raw, "\n")
		if text := strings.Join(strings.Fields(frame), " "); text != "" {
			shown = text
		}
		if step == len(steps) {
			if antigravityCLIAuthenticated(env) {
				return nil
			}
		} else if strings.Contains(shown, steps[step].prompt) {
			if keys := steps[step].answer(frame); keys != "" {
				if _, err := io.WriteString(tty, keys); err != nil {
					return fmt.Errorf("answer Antigravity sign-in: %w", err)
				}
				step, answered, shown = step+1, time.Now(), ""
			}
		}
		if (step == len(steps) || !steps[step].waitsOnBrowser) && time.Since(answered) > antigravityWizardStall {
			return fmt.Errorf("Antigravity sign-in stopped at a screen Jaz cannot answer: %s", shown)
		}
		size.Cols ^= 1
		_ = pty.Setsize(tty, &size)
	}
}
