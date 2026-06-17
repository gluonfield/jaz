package devices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wins/jaz/backend/internal/serverconfig"
)

func TestConnectionLinkUsesPublicURL(t *testing.T) {
	handler := NewHandler(nil, serverconfig.Config{Addr: ":5299", PublicURL: "https://jaz.example.com/app"})
	res := httptest.NewRecorder()
	handler.ConnectionLink(res, httptest.NewRequest(http.MethodGet, "/v1/devices/connection-link", nil))

	var got connectionLinkResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://jaz.example.com" {
		t.Fatalf("url = %q", got.URL)
	}
}

func TestConnectionLinkFallsBackToRequestHost(t *testing.T) {
	handler := NewHandler(nil, serverconfig.Config{Addr: ":5299"})
	req := httptest.NewRequest(http.MethodGet, "/v1/devices/connection-link", nil)
	req.Host = "jaz.example.net:5299"
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()
	handler.ConnectionLink(res, req)

	var got connectionLinkResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://jaz.example.net:5299" {
		t.Fatalf("url = %q", got.URL)
	}
}
