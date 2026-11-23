package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/machinebridge"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	machineBridgeSettingsNamespace = "machinebridge"
	machineBridgeDeviceIDKey       = "device_id"
)

// MachineBridgeSupervisor automatically maintains the outbound machinebridge
// WebSocket connection to Jaz Cloud while signed in (strictly as a tool host
// for Cloud agents). It reconnects with backoff when Cloud is unreachable and
// disconnects cleanly on sign-out or server shutdown.
type MachineBridgeSupervisor struct {
	cloud      *cloudsync.Service
	store      storage.SettingsStorage
	workspace  string
	log        *log.Logger
	minBackoff time.Duration
	maxBackoff time.Duration

	mu      sync.Mutex
	baseCtx context.Context
	running bool
	url     string
	token   string
	cancel  context.CancelFunc
	done    chan struct{}
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
	cancel := s.cancel
	done := s.done
	s.url = ""
	s.token = ""
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

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

func (s *MachineBridgeSupervisor) Reconcile(ctx context.Context) {
	baseURL, token, ok := s.cloud.CloudAuth(ctx)

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	if !ok {
		oldCancel, oldDone := s.cancel, s.done
		s.url, s.token, s.cancel, s.done = "", "", nil, nil
		s.mu.Unlock()
		stopBridgeLoop(oldCancel, oldDone)
		return
	}
	if s.cancel != nil && s.url == baseURL && s.token == token {
		s.mu.Unlock()
		return
	}
	oldCancel, oldDone := s.cancel, s.done
	s.url, s.token, s.cancel, s.done = "", "", nil, nil
	baseCtx := s.baseCtx
	s.mu.Unlock()

	// Wait for any previous connection with this device ID to close cleanly
	// before dialing so Cloud's MachineHub does not reject with device_already_online.
	stopBridgeLoop(oldCancel, oldDone)

	client, deviceID, err := s.newClient(baseURL, token)
	if err != nil {
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
	curURL, curToken, curOK := s.cloud.CloudAuth(ctx)
	if !curOK || curURL != baseURL || curToken != token {
		s.mu.Unlock()
		_ = client.Close()
		return
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	s.url = baseURL
	s.token = token
	s.cancel = cancel
	s.done = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		if err := client.Run(runCtx); err != nil && runCtx.Err() == nil {
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

func (s *MachineBridgeSupervisor) newClient(baseURL, token string) (*machinebridge.Client, string, error) {
	deviceID := s.resolveDeviceID()
	displayName := deviceID
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		displayName = strings.TrimSpace(host)
	}
	var grants []machinebridge.FolderGrant
	if s.workspace != "" {
		if info, err := os.Stat(s.workspace); err == nil && info.IsDir() {
			grants = append(grants, machinebridge.FolderGrant{
				ID:       "workspace",
				Mode:     machinebridge.AccessReadWrite,
				RootPath: s.workspace,
			})
		}
	}
	client, err := machinebridge.NewClient(machinebridge.ClientConfig{
		CloudURL:               baseURL,
		Token:                  token,
		DeviceID:               deviceID,
		DisplayName:            displayName,
		Capabilities:           machinebridge.Capabilities{FileStaging: true},
		FolderGrants:           grants,
		AllowLoopbackTransfers: isLoopbackBaseURL(baseURL),
		MinBackoff:             s.minBackoff,
		MaxBackoff:             s.maxBackoff,
		OnConnect: func(caps machinebridge.Capabilities) {
			s.log.Info("connected to jaz cloud machine bridge", "device_id", deviceID, "file_staging", caps.FileStaging)
		},
		OnDisconnect: func(err error) {
			s.log.Debug("disconnected from jaz cloud machine bridge; will retry", "device_id", deviceID, "error", err)
		},
	})
	if err != nil {
		return nil, "", err
	}
	return client, deviceID, nil
}

func (s *MachineBridgeSupervisor) resolveDeviceID() string {
	if s.store != nil {
		if setting, err := s.store.LoadSetting(machineBridgeSettingsNamespace, machineBridgeDeviceIDKey); err == nil {
			var id string
			if json.Unmarshal(setting.Value, &id) == nil && strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id)
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
	return id
}

func isLoopbackBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
