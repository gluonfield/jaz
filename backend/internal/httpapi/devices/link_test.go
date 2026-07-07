package devices

import (
	"testing"

	"github.com/wins/jaz/backend/internal/serverconfig"
)

func TestConnectionBaseURLKeepsConfiguredPublicURL(t *testing.T) {
	got := connectionBaseURL(
		serverconfig.Config{Addr: ":5299", PublicURL: "https://jaz.example.com/app"},
		"http://127.0.0.1:5299",
		fixedLocalIP("192.168.1.20"),
	)
	if got != "https://jaz.example.com" {
		t.Fatalf("base url = %q", got)
	}
}

func TestConnectionBaseURLKeepsNonLoopbackRequestHost(t *testing.T) {
	got := connectionBaseURL(
		serverconfig.Config{Addr: ":5299"},
		"https://jaz.example.net:5299",
		fixedLocalIP("192.168.1.20"),
	)
	if got != "https://jaz.example.net:5299" {
		t.Fatalf("base url = %q", got)
	}
}

func TestConnectionBaseURLUsesLocalNetworkIPForLoopbackRequest(t *testing.T) {
	got := connectionBaseURL(
		serverconfig.Config{Addr: ":5299"},
		"http://127.0.0.1:5299",
		fixedLocalIP("192.168.1.20"),
	)
	if got != "http://192.168.1.20:5299" {
		t.Fatalf("base url = %q", got)
	}
}

func TestConnectionBaseURLKeepsLoopbackWhenServerIsLoopbackOnly(t *testing.T) {
	got := connectionBaseURL(
		serverconfig.Config{Addr: "127.0.0.1:5299"},
		"http://127.0.0.1:5299",
		fixedLocalIP("192.168.1.20"),
	)
	if got != "http://127.0.0.1:5299" {
		t.Fatalf("base url = %q", got)
	}
}

func fixedLocalIP(ip string) func() (string, bool) {
	return func() (string, bool) { return ip, true }
}
