package whatsapp

import (
	"sync"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var configureDevicePropsOnce sync.Once

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
