package managedtool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (m *Manager) prepareMuse(ctx context.Context) error {
	m.installMu.Lock()
	defer m.installMu.Unlock()
	if status := m.Status(MuseCLI); status.State == StateReady {
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
	cmd.Stdin = strings.NewReader(string(body))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install Muse Code: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !fileExists(filepath.Join(staging, ExecutableName(MuseCLI))) {
		return fmt.Errorf("Muse installer finished without installing %s", ExecutableName(MuseCLI))
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(staging, dir)
}
