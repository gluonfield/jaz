package managedtool

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wins/jaz/backend/internal/musecli"
	"github.com/wins/jaz/backend/internal/subprocess"
)

func (m *Manager) museStatus(platform string, overrides map[string]string) Status {
	env := map[string]string{}
	maps.Copy(env, overrides)
	env["HOME"] = os.Getenv("HOME")
	path := ExecutablePath(m.root, MuseCLI)
	if selected, err := musecli.Resolve(env, filepath.Dir(path)); err == nil {
		return Status{Tool: MuseCLI, Platform: platform, Path: selected, State: StateReady}
	}
	if status, ok := m.storedStatus(MuseCLI); ok && status.State != StateReady {
		return status
	}
	return Status{Tool: MuseCLI, Platform: platform, Path: path, State: StateMissing}
}

func (m *Manager) prepareMuse(ctx context.Context, env map[string]string) error {
	m.installMu.Lock()
	defer m.installMu.Unlock()
	if status := m.Status(MuseCLI, env); status.State == StateReady {
		return nil
	}
	platform, err := platformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	spec := toolSpec{Tool: MuseCLI, Platform: platform, Command: ExecutablePath(m.root, MuseCLI)}
	m.setStatus(MuseCLI, downloadingStatus(spec))
	err = m.installMuse(ctx, filepath.Dir(spec.Command))
	if err != nil {
		m.setStatus(MuseCLI, failedStatus(spec, err))
		return err
	}
	m.setStatus(MuseCLI, readyStatus(spec))
	return nil
}

func (m *Manager) installMuse(ctx context.Context, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".muse-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	url := m.museInstallerURL
	command := "bash"
	args := []string{"-s"}
	if runtime.GOOS == "windows" {
		url = strings.TrimSuffix(url, ".sh") + ".ps1"
		command = "powershell.exe"
		args = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-"}
	}
	body, err := m.download(ctx, url)
	if err != nil {
		return fmt.Errorf("download Muse installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), "MUSE_INSTALL_DIR="+staging, "MUSE_NO_MODIFY_PATH=1", "MUSE_LOGIN=0")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	process := subprocess.New(cmd)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return err
	}
	defer process.Terminate()
	if err := process.Started(); err != nil {
		_ = process.Terminate()
		_ = cmd.Wait()
		return err
	}
	// Installers may spawn children as soon as they receive the script.
	go func() {
		_, _ = stdin.Write(body)
		_ = stdin.Close()
	}()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("install Muse Code: %w: %s", err, strings.TrimSpace(output.String()))
	}
	if !fileExists(filepath.Join(staging, ExecutableName(MuseCLI))) {
		return fmt.Errorf("Muse installer finished without installing %s", ExecutableName(MuseCLI))
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(staging, dir)
}
