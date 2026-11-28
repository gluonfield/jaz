package machinebridge

import (
	"slices"
	"testing"
)

func TestNegotiateHelloAndCapabilities(t *testing.T) {
	serverCaps := Capabilities{
		FileStaging:   true,
		LocalExec:     true,
		LocalBrowser:  false,
		ComputerUse:   true,
		LocalMCPs:     []string{"whatsapp", "browser"},
		MaxStageBytes: 50 << 20,
	}
	clientCaps := Capabilities{
		FileStaging:   true,
		LocalExec:     false,
		LocalBrowser:  true,
		ComputerUse:   true,
		LocalMCPs:     []string{"whatsapp", "unlisted", "whatsapp"},
		MaxStageBytes: 100 << 20,
	}

	resp := NegotiateHello(HelloRequest{
		ProtocolVersion: 2,
		DeviceID:        "dev-mac-1",
		DisplayName:     "Work Mac",
		Platform:        PlatformDarwin,
		Capabilities:    clientCaps,
	}, CurrentProtocolVersion, serverCaps)
	if !resp.Accepted {
		t.Fatalf("expected accepted hello, got error %q", resp.Error)
	}
	if resp.ProtocolVersion != CurrentProtocolVersion {
		t.Fatalf("version = %d, want %d", resp.ProtocolVersion, CurrentProtocolVersion)
	}
	if !resp.Capabilities.FileStaging || resp.Capabilities.LocalExec || resp.Capabilities.LocalBrowser || !resp.Capabilities.ComputerUse {
		t.Fatalf("unexpected negotiated capabilities: %+v", resp.Capabilities)
	}
	if resp.Capabilities.MaxStageBytes != 50<<20 {
		t.Fatalf("max stage bytes = %d, want %d", resp.Capabilities.MaxStageBytes, 50<<20)
	}
	if !slices.Equal(resp.Capabilities.LocalMCPs, []string{"whatsapp"}) {
		t.Fatalf("local mcps = %v, want [whatsapp]", resp.Capabilities.LocalMCPs)
	}

	if bad := NegotiateHello(HelloRequest{ProtocolVersion: 1, DeviceID: "dev-win", Platform: PlatformWindows}, CurrentProtocolVersion, serverCaps); !bad.Accepted {
		t.Fatalf("windows platform should be accepted: %+v", bad)
	}
	for _, invalid := range []HelloRequest{
		{ProtocolVersion: 0, DeviceID: "dev-1", Platform: PlatformDarwin},
		{ProtocolVersion: 1, DeviceID: "", Platform: PlatformDarwin},
		{ProtocolVersion: 1, DeviceID: "dev-1", Platform: "plan9"},
	} {
		if got := NegotiateHello(invalid, CurrentProtocolVersion, serverCaps); got.Accepted {
			t.Errorf("NegotiateHello(%+v) unexpectedly accepted", invalid)
		}
	}
}
