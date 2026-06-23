package browserworker

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (b *LocalBackend) launchBrowser(ctx context.Context) (*localBrowser, error) {
	chrome, err := b.chromePath()
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(b.Root)
	if root == "" {
		return nil, errors.New("browser runtime root is not configured")
	}
	profile := filepath.Join(root, "profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, err
	}
	portPath := filepath.Join(profile, devToolsPortFile)
	_ = os.Remove(portPath)
	cmd := exec.Command(chrome, chromeArgs(profile)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
	port, err := waitDevToolsPort(ctx, portPath, done)
	if err != nil {
		stop()
		return nil, err
	}
	if _, err := browserVersion(ctx, b.httpClient(), port); err != nil {
		stop()
		return nil, err
	}
	return &localBrowser{port: port, stop: stop}, nil
}

func stopLocalBrowser(browser *localBrowser) {
	if browser != nil && browser.stop != nil {
		browser.stop()
	}
}

func (b *LocalBackend) chromePath() (string, error) {
	path := strings.TrimSpace(b.ChromePath)
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
		return path, nil
	}
	return findChrome()
}

func waitDevToolsPort(ctx context.Context, path string, done <-chan error) (string, error) {
	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			port := strings.TrimSpace(lines[0])
			if port != "" {
				return port, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-done:
			return "", errors.New("browser exited before DevTools became available")
		case <-deadline:
			return "", errors.New("timed out waiting for browser DevTools port")
		case <-ticker.C:
		}
	}
}

func findChrome() (string, error) {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		for _, base := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Chromium", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
	default:
		candidates = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser", "microsoft-edge"}
	}
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("Chrome or Chromium executable not found")
}

func chromeArgs(profile string) []string {
	args := []string{
		"--remote-debugging-port=0",
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1280,900",
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		args = append(args, "--headless=new")
	}
	return append(args, "about:blank")
}
