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

func TestQRServiceCloseDelegatesAndForgetsSession(t *testing.T) {
	var closed string
	service := NewQRService(fakeQRProvider{provider: "matrix", closed: &closed})
	start, err := service.Start(context.Background(), "matrix")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(context.Background(), start.SessionID); err != nil {
		t.Fatal(err)
	}
	if closed != start.SessionID {
		t.Fatalf("closed session = %q, want %q", closed, start.SessionID)
	}
	_, err = service.Status(context.Background(), start.SessionID)
	if !errors.Is(err, ErrQRSessionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestQRServicePrunesTerminalSessions(t *testing.T) {
	service := NewQRService(fakeQRProvider{provider: "matrix", status: "connected"})
	start, err := service.Start(context.Background(), "matrix")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Status(context.Background(), start.SessionID); err != nil {
		t.Fatal(err)
	}
	_, err = service.Status(context.Background(), start.SessionID)
	if !errors.Is(err, ErrQRSessionNotFound) {
		t.Fatalf("err = %v", err)
	}
}

type fakeQRProvider struct {
	provider string
	expires  time.Time
	status   string
	closed   *string
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
	status := p.status
	if status == "" {
		status = "pending"
	}
	return QRStatus{
		Provider: p.provider,
		Status:   status,
	}, nil
}

func (p fakeQRProvider) CloseQR(_ context.Context, id string) error {
	if p.closed != nil {
		*p.closed = id
	}
	return nil
}
