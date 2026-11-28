package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/gluonfield/acp-transport/stdio"
	"github.com/wins/jaz/backend/internal/processenv"
	"github.com/wins/jaz/backend/internal/subprocess"
)

const antigravityAccountGrace = 250 * time.Millisecond

// AntigravityCloudLogin is a Google Cloud sign-in: a Gemini Enterprise
// license, found from the account or from the project it is assigned in, or
// pay-as-you-go usage billed to a project.
type AntigravityCloudLogin struct {
	Project    string `json:"project"`
	Location   string `json:"location"`
	PayAsYouGo bool   `json:"pay_as_you_go"`
}

// AntigravityBrowser connects a sign-in to the user's browser: Page receives
// the sign-in page once agy shows it, and Codes carries a code that page told
// the user to paste back.
type AntigravityBrowser struct {
	Page  func(url string)
	Codes <-chan string
}

// LoginAntigravityCloud returns once the user has finished signing in. agy
// opens the browser itself on the host it runs on; only the pay-as-you-go
// wizard also reports its page and takes a code, so only it works from another
// machine.
func LoginAntigravityCloud(ctx context.Context, cfg AgentConfig, root string, login AntigravityCloudLogin, browser AntigravityBrowser) error {
	if login.Project != "" && login.Location == "" {
		return errors.New("a Google Cloud project needs a location")
	}
	if login.PayAsYouGo {
		if login.Project == "" {
			return errors.New("pay-as-you-go needs a Google Cloud project")
		}
		return runAntigravityWizard(ctx, cfg, root, antigravityPayAsYouGoSteps(login), browser)
	}
	if login.Project != "" {
		if err := saveAntigravityCloudProject(login); err != nil {
			return err
		}
	}
	return withAntigravityAccountPeer(ctx, cfg, root, "", func(peer *jsonrpc.Peer, _ acpschema.InitializeResponse) error {
		_, err := peer.Call(ctx, acpschema.AgentMethodAuthenticate, acpschema.AuthenticateRequest{MethodID: "oauth-business"})
		return err
	})
}

func saveAntigravityCloudProject(login AntigravityCloudLogin) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".gemini", "antigravity-cli")
	settings := map[string]json.RawMessage{}
	gcp := map[string]any{}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("read Antigravity settings: %w", err)
		}
	}
	if settings["gcp"] != nil {
		if err := json.Unmarshal(settings["gcp"], &gcp); err != nil {
			return fmt.Errorf("read Antigravity settings: %w", err)
		}
	}
	gcp["project"], gcp["location"] = login.Project, login.Location
	if settings["gcp"], err = json.Marshal(gcp); err != nil {
		return err
	}
	if raw, err = json.MarshalIndent(settings, "", "  "); err != nil {
		return err
	}
	return writeAntigravitySettings(dir, raw)
}

func writeAntigravitySettings(dir string, settings []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(settings)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, "settings.json"))
}

func antigravityLoginWorkspace(root string) string {
	return filepath.Join(root, "acp", "agy-login-workspace")
}

func disconnectAntigravityAuth(ctx context.Context, cfg AgentConfig, root string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := withAntigravityAccountPeer(ctx, cfg, root, "", func(peer *jsonrpc.Peer, init acpschema.InitializeResponse) error {
		caps := init.AgentCapabilities
		if caps == nil || caps.Auth == nil || caps.Auth.Logout == nil {
			return fmt.Errorf("Antigravity does not advertise native sign out support")
		}
		if _, err := peer.Call(ctx, acpschema.AgentMethodLogout, acpschema.LogoutRequest{}); err != nil {
			return fmt.Errorf("Antigravity sign out: %w", err)
		}
		return nil
	})
	if err == nil {
		invalidateAntigravityAuthCache()
	}
	return err
}

func withAntigravityAccountPeer(ctx context.Context, cfg AgentConfig, root, cwd string, run func(*jsonrpc.Peer, acpschema.InitializeResponse) error) (err error) {
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	env := manager.probeEnv(AgentAntigravity, cfg)
	if cfg.ManagedAdapter != "" {
		command, err := antigravityExecutable(env["PATH"])
		if err != nil {
			return err
		}
		cfg.useCommandLaunch(command, cfg.ManagedAdapterArgs)
	}
	var conn jsonrpc.MessageConn
	if cfg.URL != "" {
		remote, _, err := manager.openConn(ctx, AgentAntigravity, cfg, env, cwd, "", nil)
		if err != nil {
			return err
		}
		conn = remote
		defer conn.Close()
	} else {
		processCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		command, args := launchCommand(cfg.Command, cfg.Args)
		addCommandDirToPath(env, command)
		cmd := exec.CommandContext(processCtx, command, args...)
		cmd.Env = processenv.List(env)
		cmd.Dir = cwd
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer stdin.Close()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		defer stdout.Close()
		stderr := newProcessStderrTail()
		cmd.Stderr = stderr
		process := subprocess.New(cmd)
		cmd.Cancel = stdin.Close
		cmd.WaitDelay = antigravityAccountGrace
		if err := cmd.Start(); err != nil {
			return err
		}
		defer func() {
			cancel()
			_ = cmd.Wait()
			_ = process.TerminateWithin(antigravityAccountGrace)
			if detail, _ := stderr.snapshot(); err != nil && detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
		}()
		if err := process.Started(); err != nil {
			return err
		}
		conn = stdio.New(stdout, stdin)
	}
	peer := jsonrpc.NewPeer(conn, nil)
	go func() {
		_ = peer.Serve(ctx)
	}()
	raw, err := peer.Call(ctx, acpschema.AgentMethodInitialize, acpschema.InitializeRequest{
		ProtocolVersion: acpschema.ProtocolVersionNumber,
		ClientInfo:      &acpschema.Implementation{Name: "jaz", Title: "Jaz"},
	})
	if err != nil {
		return err
	}
	var initialized acpschema.InitializeResponse
	if err := json.Unmarshal(raw, &initialized); err != nil {
		return err
	}
	if initialized.ProtocolVersion != acpschema.ProtocolVersionNumber {
		return fmt.Errorf("unsupported ACP protocol version %d", initialized.ProtocolVersion)
	}
	return run(peer, initialized)
}
