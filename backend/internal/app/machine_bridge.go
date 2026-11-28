package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acpcredentials"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/machinebridge"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	machineBridgeSettingsNamespace = "machinebridge"
	machineBridgeDeviceIDKey       = "device_id"
	machineBridgeConfigKey         = "config"
)

// MachineBridgeSupervisor automatically maintains the outbound machinebridge
// WebSocket connection to Jaz Cloud while signed in (strictly as a tool host
// for Cloud agents). It reconnects with backoff when Cloud is unreachable and
// disconnects cleanly on sign-out, pause, config updates, or server shutdown.
type MachineBridgeSupervisor struct {
	cloud      *cloudsync.Service
	store      storage.SettingsStorage
	workspace  string
	log        *log.Logger
	minBackoff time.Duration
	maxBackoff time.Duration

	deviceIDOnce sync.Once
	deviceID     string

	opMu            sync.Mutex
	mu              sync.Mutex
	baseCtx         context.Context
	running         bool
	connGen         uint64
	selection       cloudsync.AccountSelection
	client          *machinebridge.Client
	url             string
	token           string
	configHash      string
	connected       bool
	lastConnectedAt *time.Time
	lastError       string
	cancel          context.CancelFunc
	done            chan struct{}
}

func NewMachineBridgeSupervisor(cloud *cloudsync.Service, store storage.SettingsStorage, workspace string, logger *log.Logger) *MachineBridgeSupervisor {
	if logger == nil {
		logger = log.Default()
	}
	return &MachineBridgeSupervisor{
		cloud:     cloud,
		store:     store,
		workspace: strings.TrimSpace(workspace),
		log:       logger.WithPrefix("machinebridge"),
	}
}

func (s *MachineBridgeSupervisor) Start(ctx context.Context) {
	s.mu.Lock()
	s.baseCtx = ctx
	s.running = true
	s.mu.Unlock()
	s.Reconcile(ctx)
}

func (s *MachineBridgeSupervisor) Stop(ctx context.Context) {
	s.mu.Lock()
	s.running = false
	s.connGen++
	s.connected = false
	earlyCancel := s.cancel
	s.mu.Unlock()
	if earlyCancel != nil {
		earlyCancel()
	}

	s.opMu.Lock()
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.url = ""
	s.token = ""
	s.configHash = ""
	s.connected = false
	s.cancel = nil
	s.client = nil
	s.done = nil
	s.mu.Unlock()
	s.opMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}

func (s *MachineBridgeSupervisor) Status() machinebridge.BridgeStatus {
	deviceID := s.resolveDeviceID()
	cfg := s.loadConfig(deviceID)

	s.mu.Lock()
	connected := s.connected
	cloudURL := s.url
	lastError := s.lastError
	var lastConnectedAt *time.Time
	if s.lastConnectedAt != nil {
		copied := *s.lastConnectedAt
		lastConnectedAt = &copied
	}
	s.mu.Unlock()

	if cloudURL == "" {
		if activeURL, _, ok := s.cloud.CloudAuth(context.Background()); ok {
			cloudURL = activeURL
		}
	}

	return machinebridge.BridgeStatus{
		DeviceID:        deviceID,
		Connected:       connected,
		CloudURL:        cloudURL,
		LastConnectedAt: lastConnectedAt,
		LastError:       lastError,
		Config:          cfg.Clone(),
	}
}

func (s *MachineBridgeSupervisor) UpdateConfig(ctx context.Context, cfg machinebridge.ShareConfig) (machinebridge.BridgeStatus, error) {
	deviceID := s.resolveDeviceID()
	normalized, err := machinebridge.NormalizeShareConfig(cfg, deviceID)
	if err != nil {
		return machinebridge.BridgeStatus{}, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return machinebridge.BridgeStatus{}, err
	}

	s.opMu.Lock()
	if s.store != nil {
		if _, err := s.store.SaveSetting(machineBridgeSettingsNamespace, machineBridgeConfigKey, encoded); err != nil {
			s.opMu.Unlock()
			return machinebridge.BridgeStatus{}, err
		}
	}
	s.reconcileLocked(ctx)
	s.opMu.Unlock()

	return s.Status(), nil
}

func (s *MachineBridgeSupervisor) Reconcile(ctx context.Context) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.reconcileLocked(ctx)
}

func (s *MachineBridgeSupervisor) reconcileLocked(ctx context.Context) {
	account, selection := s.cloud.ActiveCloudAccount()
	baseURL, token := account.BaseURL, account.Token
	deviceID := s.resolveDeviceID()
	cfg := s.loadConfig(deviceID)
	cfgHash := cfg.Hash()

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	if selection.ID == "" || !cfg.Enabled {
		oldCancel, oldDone := s.cancel, s.done
		s.connGen++
		s.url, s.token, s.configHash, s.cancel, s.done = "", "", "", nil, nil
		s.connected = false
		s.client = nil
		s.lastError = ""
		s.mu.Unlock()
		stopBridgeLoop(oldCancel, oldDone)
		return
	}
	if s.cancel != nil && s.selection == selection && s.url == baseURL && s.token == token && s.configHash == cfgHash {
		s.mu.Unlock()
		return
	}
	oldCancel, oldDone := s.cancel, s.done
	s.connGen++
	gen := s.connGen
	s.url, s.token, s.configHash, s.cancel, s.done = "", "", "", nil, nil
	s.connected = false
	s.client = nil
	s.lastError = ""
	baseCtx := s.baseCtx
	s.mu.Unlock()

	// Wait for any previous connection with this device ID to close cleanly
	// before dialing so Cloud's MachineHub does not reject with device_already_online.
	stopBridgeLoop(oldCancel, oldDone)

	client, err := s.newClient(baseURL, token, deviceID, cfg, gen)
	if err != nil {
		s.mu.Lock()
		if s.connGen == gen {
			s.lastError = err.Error()
		}
		s.mu.Unlock()
		s.log.Warn("machinebridge client setup failed", "error", err)
		return
	}

	s.mu.Lock()
	if !s.running || baseCtx.Err() != nil {
		s.mu.Unlock()
		_ = client.Close()
		return
	}
	// Verify account didn't change again while we were stopping/building.
	current, currentSelection := s.cloud.ActiveCloudAccount()
	if currentSelection != selection || current.Token != token {
		s.mu.Unlock()
		_ = client.Close()
		return
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	s.selection = selection
	s.client = client
	s.url = baseURL
	s.token = token
	s.configHash = cfgHash
	s.cancel = cancel
	s.done = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		if err := client.Run(runCtx); err != nil && runCtx.Err() == nil {
			s.mu.Lock()
			if s.connGen == gen {
				s.connected = false
				s.lastError = err.Error()
			}
			s.mu.Unlock()
			s.log.Warn("machinebridge stopped", "device_id", deviceID, "error", err)
		}
	}()
}

func stopBridgeLoop(cancel context.CancelFunc, done chan struct{}) {
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *MachineBridgeSupervisor) newClient(baseURL, token, deviceID string, cfg machinebridge.ShareConfig, gen uint64) (*machinebridge.Client, error) {
	var execFactory func(*machinebridge.PolicyGuard) (machinebridge.ExecLauncher, error)
	if cfg.AllowExec {
		execFactory = func(guard *machinebridge.PolicyGuard) (machinebridge.ExecLauncher, error) {
			return machinebridge.NewCodexExecLauncher(machinebridge.CodexExecConfig{Guard: guard})
		}
	}
	client, err := machinebridge.NewClient(machinebridge.ClientConfig{
		CloudURL:    baseURL,
		Token:       token,
		DeviceID:    deviceID,
		DisplayName: cfg.DisplayName,
		Capabilities: machinebridge.Capabilities{
			ACPProfiles: true,
			FileStaging: true,
			LocalExec:   cfg.AllowExec,
		},
		FolderGrants:           cfg.Grants,
		AllowLoopbackTransfers: cloudsync.IsLoopbackURL(baseURL),
		ExecLauncherFactory:    execFactory,
		MinBackoff:             s.minBackoff,
		MaxBackoff:             s.maxBackoff,
		OnConnect: func(caps machinebridge.Capabilities) {
			now := time.Now().UTC()
			s.mu.Lock()
			if s.connGen == gen && s.running {
				s.connected = true
				s.lastConnectedAt = &now
				s.lastError = ""
			}
			s.mu.Unlock()
			s.log.Info("connected to jaz cloud machine bridge", "device_id", deviceID, "file_staging", caps.FileStaging, "local_exec", caps.LocalExec)
		},
		OnDisconnect: func(err error) {
			s.mu.Lock()
			if s.connGen == gen {
				s.connected = false
				if err != nil {
					s.lastError = err.Error()
				}
			}
			s.mu.Unlock()
			s.log.Debug("disconnected from jaz cloud machine bridge; will retry", "device_id", deviceID, "error", err)
		},
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (s *MachineBridgeSupervisor) loadConfig(deviceID string) machinebridge.ShareConfig {
	if s.store != nil {
		if setting, err := s.store.LoadSetting(machineBridgeSettingsNamespace, machineBridgeConfigKey); err == nil {
			var saved machinebridge.ShareConfig
			if json.Unmarshal(setting.Value, &saved) == nil {
				if strings.TrimSpace(saved.DisplayName) == "" {
					saved.DisplayName = machinebridge.DefaultDisplayName(deviceID)
				}
				if saved.Grants == nil {
					saved.Grants = []machinebridge.FolderGrant{}
				}
				return saved
			}
		}
	}
	return machinebridge.DefaultShareConfig(s.workspace, deviceID)
}

func (s *MachineBridgeSupervisor) resolveDeviceID() string {
	s.deviceIDOnce.Do(func() {
		if s.store != nil {
			if setting, err := s.store.LoadSetting(machineBridgeSettingsNamespace, machineBridgeDeviceIDKey); err == nil {
				var id string
				if json.Unmarshal(setting.Value, &id) == nil && strings.TrimSpace(id) != "" {
					s.deviceID = strings.TrimSpace(id)
					return
				}
			}
		}
		var raw [8]byte
		_, _ = rand.Read(raw[:])
		id := "jaz-" + hex.EncodeToString(raw[:])
		if s.store != nil {
			if encoded, err := json.Marshal(id); err == nil {
				_, _ = s.store.SaveSetting(machineBridgeSettingsNamespace, machineBridgeDeviceIDKey, encoded)
			}
		}
		s.deviceID = id
	})
	return s.deviceID
}

func (s *MachineBridgeSupervisor) ResolveACPProfile(ctx context.Context, workspaceID, profileID string, rejectedVersion int64) (acpcredentials.Profile, error) {
	s.mu.Lock()
	client, selection, gen := s.client, s.selection, s.connGen
	s.mu.Unlock()
	_, active := s.cloud.ActiveCloudAccount()
	if active.ID != workspaceID || workspaceID == "" {
		return acpcredentials.Profile{}, cloudsync.ErrWorkspaceChanged
	}
	if client == nil || active != selection {
		return acpcredentials.Profile{}, machinebridge.ErrDeviceOffline
	}
	profile, err := client.ResolveACPProfile(ctx, profileID, rejectedVersion)
	s.mu.Lock()
	sameClient := s.connGen == gen
	s.mu.Unlock()
	_, active = s.cloud.ActiveCloudAccount()
	if !sameClient || active != selection {
		return acpcredentials.Profile{}, cloudsync.ErrWorkspaceChanged
	}
	return profile, err
}
