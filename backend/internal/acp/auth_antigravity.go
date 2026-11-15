package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func disconnectAntigravityAuth(ctx context.Context, cfg AgentConfig, root string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if cfg.ManagedAdapter != "" {
		command, err := antigravityExecutable(loginBinDirs(cfg))
		if err != nil {
			return err
		}
		cfg.useCommandLaunch(command, cfg.ManagedAdapterArgs)
	}
	manager := NewManager(nil, Config{Root: root}, nil)
	defer manager.Close()
	conn, err := manager.connect(ctx, AgentAntigravity, cfg, "", "", "", nil)
	if err != nil {
		return err
	}
	defer conn.close()
	var initialized acpschema.InitializeResponse
	if err := json.Unmarshal(conn.initRaw, &initialized); err != nil {
		return err
	}
	capabilities := initialized.AgentCapabilities
	if capabilities == nil || capabilities.Auth == nil || capabilities.Auth.Logout == nil {
		return fmt.Errorf("Antigravity does not advertise native sign out support")
	}
	if _, err := conn.peer.Call(ctx, acpschema.AgentMethodLogout, acpschema.LogoutRequest{}); err != nil {
		return fmt.Errorf("Antigravity sign out: %w", conn.withProcessStderr(err))
	}
	invalidateAntigravityAuthCache()
	return nil
}
