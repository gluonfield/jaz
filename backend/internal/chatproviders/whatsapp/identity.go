package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var refreshWebVersion = versionRefresh{}

const (
	whatsappCompanionName       = "Chrome (Linux)"
	whatsappWebServiceWorkerURL = "https://web.whatsapp.com/sw.js"
)

type versionRefresh struct {
	mu     sync.Mutex
	loaded bool
}

func newWhatsAppClient(device *store.Device) *whatsmeow.Client {
	configureWhatsAppDeviceProps()
	client := whatsmeow.NewClient(device, waLog.Noop)
	client.QRClientType = whatsmeow.PairClientChrome
	return client
}

func configureWhatsAppDeviceProps() {
	version := store.GetWAVersion()
	store.DeviceProps.Os = proto.String(whatsappCompanionName)
	store.DeviceProps.Version = &waCompanionReg.DeviceProps_AppVersion{
		Primary:   proto.Uint32(version[0]),
		Secondary: proto.Uint32(version[1]),
		Tertiary:  proto.Uint32(version[2]),
	}
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
}

func prepareWhatsAppClient(ctx context.Context) error {
	if err := refreshWhatsAppWebVersion(ctx); err != nil {
		return err
	}
	configureWhatsAppDeviceProps()
	return nil
}

func refreshWhatsAppWebVersion(ctx context.Context) error {
	refreshWebVersion.mu.Lock()
	if refreshWebVersion.loaded {
		refreshWebVersion.mu.Unlock()
		return nil
	}
	refreshWebVersion.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := latestWhatsAppWebVersion(ctx, nil)
	if err != nil {
		return err
	}
	store.SetWAVersion(*version)

	refreshWebVersion.mu.Lock()
	refreshWebVersion.loaded = true
	refreshWebVersion.mu.Unlock()
	return nil
}

func latestWhatsAppWebVersion(ctx context.Context, client *http.Client) (*store.WAVersionContainer, error) {
	version, err := whatsmeow.GetLatestVersion(ctx, client)
	if err == nil {
		return version, nil
	}
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, whatsappWebServiceWorkerURL, nil)
	if reqErr != nil {
		return nil, reqErr
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, reqErr := client.Do(req)
	if reqErr != nil {
		return nil, fmt.Errorf("fallback WhatsApp Web version request failed after main page parse failed: %w", reqErr)
	}
	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fallback WhatsApp Web version request returned %d", resp.StatusCode)
	}
	version, parseErr := parseWhatsAppWebVersion(data)
	if parseErr != nil {
		return nil, fmt.Errorf("fallback WhatsApp Web version parse failed after main page parse failed: %w", parseErr)
	}
	return version, nil
}

func parseWhatsAppWebVersion(data []byte) (*store.WAVersionContainer, error) {
	const key = "client_revision"
	idx := bytes.Index(data, []byte(key))
	if idx < 0 {
		return nil, fmt.Errorf("%s not found", key)
	}
	tail := data[idx+len(key):]
	colon := bytes.IndexByte(tail, ':')
	if colon < 0 {
		return nil, fmt.Errorf("%s value not found", key)
	}
	tail = tail[colon+1:]
	start := 0
	for start < len(tail) && (tail[start] < '0' || tail[start] > '9') {
		start++
	}
	end := start
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if start == end {
		return nil, fmt.Errorf("%s numeric value not found", key)
	}
	revision, err := strconv.ParseUint(string(tail[start:end]), 10, 32)
	if err != nil {
		return nil, err
	}
	return &store.WAVersionContainer{2, 3000, uint32(revision)}, nil
}
