package mobilepush

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type appleTransport struct {
	base http.RoundTripper
	host string
	t    *testing.T
}

func (r appleTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.t.Helper()
	if request.URL.Host != "api.sandbox.push.apple.com" {
		r.t.Errorf("wrong Apple environment: %s", request.URL.Host)
	}
	next := request.Clone(request.Context())
	next.URL.Host = r.host
	next.Host = r.host
	return r.base.RoundTrip(next)
}

func TestAppleHTTP2PayloadAndSignedProviderToken(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "AuthKey.p8")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.ProtoMajor != 2 || r.Method != http.MethodPost || r.URL.Path != "/3/device/"+strings.Repeat("ab", 32) {
			t.Errorf("wrong APNs request: %s %s %s", r.Proto, r.Method, r.URL.Path)
		}
		parts := strings.Split(strings.TrimPrefix(r.Header.Get("authorization"), "bearer "), ".")
		if len(parts) != 3 {
			t.Error("missing JWT")
			w.WriteHeader(403)
			return
		}
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil || len(signature) != 64 {
			t.Error("invalid ES256 signature format")
			w.WriteHeader(403)
			return
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
			t.Error("provider signature does not verify")
		}
		claimsBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims struct {
			Issuer string `json:"iss"`
			Issued int64  `json:"iat"`
		}
		if err := json.Unmarshal(claimsBytes, &claims); err != nil || claims.Issuer != "TEAM" || time.Since(time.Unix(claims.Issued, 0)) > time.Minute {
			t.Errorf("invalid provider claims: %s", claimsBytes)
		}
		if r.Header.Get("apns-push-type") != "liveactivity" || r.Header.Get("apns-topic") != "com.example.app.push-type.liveactivity" || r.Header.Get("apns-priority") != "5" {
			t.Errorf("incorrect ActivityKit headers: %#v", r.Header)
		}
		var payload struct {
			APS struct {
				Event string       `json:"event"`
				State contentState `json:"content-state"`
			} `json:"aps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.APS.Event != "end" || payload.APS.State.Status != "finished" {
			t.Errorf("incorrect ActivityKit payload: %#v, %v", payload, err)
		}
		if calls == 2 {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte("{\"reason\":\"Unregistered\"}"))
			return
		}
		if calls == 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	apple, err := NewApple(Config{KeyFile: keyFile, KeyID: "KEY", TeamID: "TEAM", BundleID: "com.example.app"})
	if err != nil {
		t.Fatal(err)
	}
	apple.client = server.Client()
	apple.client.Transport = appleTransport{base: apple.client.Transport, host: strings.TrimPrefix(server.URL, "https://"), t: t}
	payload := map[string]any{"aps": map[string]any{"event": "end", "content-state": contentState{"finished", "Done"}}}
	if err := apple.Send(context.Background(), strings.Repeat("ab", 32), "development", "liveactivity", "activity", payload); err != nil {
		t.Fatal(err)
	}
	if err := apple.Send(context.Background(), strings.Repeat("ab", 32), "development", "liveactivity", "activity", payload); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unregistered token not invalidated: %v", err)
	}
	if err := apple.Send(context.Background(), strings.Repeat("ab", 32), "development", "liveactivity", "activity", payload); err != nil || calls != 4 {
		t.Fatalf("transient Apple failure was not retried: calls=%d error=%v", calls, err)
	}
}
