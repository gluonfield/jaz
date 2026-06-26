package whatsapp

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var configureDevicePropsOnce sync.Once
var refreshWebVersion = versionRefresh{}

const webVersionRefreshInterval = time.Hour

type versionRefresh struct {
	mu          sync.Mutex
	lastAttempt time.Time
	loaded      bool
}

func newWhatsAppClient(device *store.Device) *whatsmeow.Client {
	configureWhatsAppDeviceProps()
	client := whatsmeow.NewClient(device, waLog.Noop)
	client.QRClientType = whatsmeow.PairClientChrome
	return client
}

func configureWhatsAppDeviceProps() {
	configureDevicePropsOnce.Do(func() {
		store.DeviceProps.Os = proto.String("Jaz")
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	})
}

func refreshWhatsAppWebVersion(ctx context.Context) {
	refreshWebVersion.mu.Lock()
	if refreshWebVersion.loaded || time.Since(refreshWebVersion.lastAttempt) < webVersionRefreshInterval {
		refreshWebVersion.mu.Unlock()
		return
	}
	refreshWebVersion.lastAttempt = time.Now()
	refreshWebVersion.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := whatsmeow.GetLatestVersion(ctx, nil)
	if err != nil {
		return
	}
	store.SetWAVersion(*version)

	refreshWebVersion.mu.Lock()
	refreshWebVersion.loaded = true
	refreshWebVersion.mu.Unlock()
}
