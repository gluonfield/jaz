package apps

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ticketIssuer   = "jaz-local"
	ticketAudience = "jaz-app-content"
	ticketScope    = "app_content"
	ticketTTL      = 5 * time.Minute
)

type TicketClaims struct {
	AppID        string `json:"app_id"`
	Version      int    `json:"version"`
	BundleSHA256 string `json:"bundle_sha256"`
	Scope        string `json:"scope"`
	Iss          string `json:"iss"`
	Aud          string `json:"aud"`
	Nbf          int64  `json:"nbf"`
	Exp          int64  `json:"exp"`
	Iat          int64  `json:"iat"`
}

func loadOrCreateTicketKey(appsRoot string) ([]byte, error) {
	if err := os.MkdirAll(appsRoot, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(appsRoot, ".ticket-key")
	if raw, err := os.ReadFile(keyPath); err == nil && len(raw) == 32 {
		return raw, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func signHS256(key []byte, claims TicketClaims) (string, error) {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := hdr + "." + payload
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

func verifyHS256(key []byte, token string) (TicketClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TicketClaims{}, errors.New("malformed app ticket")
	}
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expected, got) {
		return TicketClaims{}, errors.New("invalid app ticket signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TicketClaims{}, errors.New("invalid app ticket payload")
	}
	var claims TicketClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return TicketClaims{}, fmt.Errorf("decode app ticket: %w", err)
	}
	if claims.Iss != ticketIssuer || claims.Aud != ticketAudience || claims.Scope != ticketScope {
		return TicketClaims{}, errors.New("invalid app ticket scope")
	}
	now := time.Now().UTC().Unix()
	if claims.Nbf <= 0 || now < claims.Nbf {
		return TicketClaims{}, errors.New("app ticket not yet valid")
	}
	if claims.Exp <= 0 || now > claims.Exp {
		return TicketClaims{}, errors.New("app ticket expired")
	}
	if !IsValidAppID(claims.AppID) || claims.Version <= 0 {
		return TicketClaims{}, errors.New("incomplete app ticket claims")
	}
	return claims, nil
}
