package connections

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQRServiceDelegatesToProvider(t *testing.T) {
	expires := time.Date(2026, 6, 26, 10, 32, 0, 0, time.UTC)
	service := NewQRService(fakeQRProvider{provider: "matrix", expires: expires})

	start, err := service.Start(context.Background(), "matrix")
	if err != nil {
		t.Fatal(err)
	}
	if start.SessionID != "qr_1" || start.Provider != "matrix" || start.Code != "matrix-login-code" || !start.ExpiresAt.Equal(expires) {
		t.Fatalf("start = %#v", start)
	}
	status, err := service.Status(context.Background(), start.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "pending" || status.Provider != "matrix" {
		t.Fatalf("status = %#v", status)
	}
}

func TestQRServiceRejectsProviderWithoutAdapter(t *testing.T) {
	_, err := NewQRService().Start(context.Background(), "whatsapp")
	if !errors.Is(err, ErrQRProviderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestQRServiceRejectsUnknownSession(t *testing.T) {
	_, err := NewQRService(fakeQRProvider{provider: "matrix"}).Status(context.Background(), "missing")
	if !errors.Is(err, ErrQRSessionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

type fakeQRProvider struct {
	provider string
	expires  time.Time
}

func (p fakeQRProvider) ProviderID() string {
	return p.provider
}

func (p fakeQRProvider) StartQR(context.Context) (QRStart, error) {
	return QRStart{
		SessionID: "qr_1",
		Provider:  p.provider,
		Code:      p.provider + "-login-code",
		Status:    "pending",
		ExpiresAt: p.expires,
	}, nil
}

func (p fakeQRProvider) QRStatus(context.Context, string) (QRStatus, error) {
	return QRStatus{
		Provider: p.provider,
		Status:   "pending",
	}, nil
}
