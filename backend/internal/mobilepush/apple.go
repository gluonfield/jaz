package mobilepush

import (
	"bytes"
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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

var ErrInvalidToken = errors.New("Apple push token is no longer valid")

type Config struct {
	KeyID    string
	TeamID   string
	KeyFile  string
	BundleID string
}

type Sender interface {
	Enabled() bool
	Send(context.Context, string, string, string, string, any) error
}

type Apple struct {
	config Config
	key    *ecdsa.PrivateKey
	client *http.Client
	mu     sync.Mutex
	jwt    string
	issued time.Time
}

func NewApple(config Config) (*Apple, error) {
	a := &Apple{config: config, client: &http.Client{Timeout: 10 * time.Second}}
	if config.KeyFile == "" && config.KeyID == "" && config.TeamID == "" {
		return a, nil
	}
	if config.KeyFile == "" || config.KeyID == "" || config.TeamID == "" || config.BundleID == "" {
		return nil, errors.New("Apple push requires KeyFile, KeyID, TeamID and BundleID")
	}
	data, err := os.ReadFile(config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("read Apple push key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("Apple push key must be a PEM PKCS8 private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse Apple push key: %w", err)
	}
	a.key, _ = key.(*ecdsa.PrivateKey)
	if a.key == nil || a.key.Curve != elliptic.P256() {
		return nil, errors.New("Apple push key must use ECDSA P-256")
	}
	return a, nil
}

func (a *Apple) Enabled() bool {
	return a.key != nil
}

func (a *Apple) authorization() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && time.Since(a.issued) < 50*time.Minute {
		return a.jwt, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": a.config.KeyID})
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{"iss": a.config.TeamID, "iat": now.Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, a.key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	a.jwt = unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	a.issued = now
	return a.jwt, nil
}

type appleFailure struct {
	status int
	reason string
}

func (e *appleFailure) Error() string {
	return fmt.Sprintf("Apple push HTTP %d: %s", e.status, e.reason)
}

func (a *Apple) Send(ctx context.Context, token, environment, pushType, collapse string, payload any) error {
	var failure error
	for attempt := range 3 {
		failure = a.send(ctx, token, environment, pushType, collapse, payload)
		if failure == nil || ctx.Err() != nil {
			return failure
		}
		var response *appleFailure
		var transport net.Error
		retry := errors.As(failure, &response) && (response.status == 429 || response.status >= 500) || errors.As(failure, &transport)
		if !retry || attempt == 2 {
			return failure
		}
		timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return failure
}

func (a *Apple) send(ctx context.Context, token, environment, pushType, collapse string, payload any) error {
	if !a.Enabled() {
		return errors.New("Apple push is not configured")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(data) > 4096 {
		return errors.New("Apple push payload exceeds 4096 bytes")
	}
	authorization, err := a.authorization()
	if err != nil {
		return err
	}
	host := "https://api.push.apple.com"
	if environment == "development" {
		host = "https://api.sandbox.push.apple.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+token, bytes.NewReader(data))
	if err != nil {
		return err
	}
	topic := a.config.BundleID
	priority := "10"
	if pushType == "liveactivity" {
		topic += ".push-type.liveactivity"
		priority = "5"
	}
	req.Header.Set("authorization", "bearer "+authorization)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("apns-topic", topic)
	req.Header.Set("apns-push-type", pushType)
	req.Header.Set("apns-priority", priority)
	req.Header.Set("apns-expiration", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	req.Header.Set("apns-collapse-id", collapse)
	response, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("Apple push transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	var failure struct{ Reason string }
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
	if response.StatusCode == http.StatusGone || failure.Reason == "BadDeviceToken" || failure.Reason == "DeviceTokenNotForTopic" {
		return ErrInvalidToken
	}
	return &appleFailure{status: response.StatusCode, reason: failure.Reason}
}
