package connections

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrQRProviderUnavailable = errors.New("connection QR provider unavailable")
	ErrQRSessionNotFound     = errors.New("connection QR session not found")
)

type QRStart struct {
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	Code         string    `json:"code"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	Instructions []string  `json:"instructions"`
}

type QRStatus struct {
	SessionID string    `json:"session_id"`
	Provider  string    `json:"provider"`
	Code      string    `json:"code,omitempty"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	AccountID string    `json:"account_id,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type QRProvider interface {
	ProviderID() string
	StartQR(context.Context) (QRStart, error)
	QRStatus(context.Context, string) (QRStatus, error)
	CloseQR(context.Context, string) error
}

type QRService struct {
	mu        sync.Mutex
	providers map[string]QRProvider
	sessions  map[string]string
}

func NewQRService(providers ...QRProvider) *QRService {
	service := &QRService{
		providers: map[string]QRProvider{},
		sessions:  map[string]string{},
	}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		service.providers[provider.ProviderID()] = provider
	}
	return service
}

func (s *QRService) Start(ctx context.Context, provider string) (QRStart, error) {
	adapter, ok := s.provider(provider)
	if !ok {
		return QRStart{}, fmt.Errorf("%w: %s", ErrQRProviderUnavailable, provider)
	}
	start, err := adapter.StartQR(ctx)
	if err != nil {
		return QRStart{}, err
	}
	if start.SessionID == "" || start.Code == "" {
		return rejectQRStart(ctx, adapter, start, fmt.Errorf("connection QR provider %q returned an incomplete session", provider))
	}
	if len(start.Instructions) == 0 {
		return rejectQRStart(ctx, adapter, start, fmt.Errorf("connection QR provider %q returned no instructions", provider))
	}
	if start.Provider == "" {
		start.Provider = provider
	}
	if start.Provider != provider {
		return rejectQRStart(ctx, adapter, start, fmt.Errorf("connection QR provider %q returned provider %q", provider, start.Provider))
	}

	s.mu.Lock()
	s.sessions[start.SessionID] = provider
	s.mu.Unlock()
	return start, nil
}

func (s *QRService) Status(ctx context.Context, id string) (QRStatus, error) {
	s.mu.Lock()
	provider := s.sessions[id]
	adapter := s.providers[provider]
	s.mu.Unlock()
	if adapter == nil {
		return QRStatus{}, ErrQRSessionNotFound
	}
	status, err := adapter.QRStatus(ctx, id)
	if err != nil {
		if errors.Is(err, ErrQRSessionNotFound) {
			s.forget(id)
		}
		return QRStatus{}, err
	}
	if status.SessionID == "" {
		status.SessionID = id
	}
	if status.Provider == "" {
		status.Provider = provider
	}
	if qrTerminal(status.Status) {
		s.forget(id)
	}
	return status, nil
}

func (s *QRService) Close(ctx context.Context, id string) error {
	s.mu.Lock()
	provider := s.sessions[id]
	adapter := s.providers[provider]
	delete(s.sessions, id)
	s.mu.Unlock()
	if adapter == nil {
		return ErrQRSessionNotFound
	}
	return adapter.CloseQR(ctx, id)
}

func (s *QRService) Available(provider string) bool {
	_, ok := s.provider(provider)
	return ok
}

func (s *QRService) provider(id string) (QRProvider, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.providers[id]
	if !ok {
		return nil, false
	}
	return provider, true
}

func rejectQRStart(ctx context.Context, adapter QRProvider, start QRStart, err error) (QRStart, error) {
	if start.SessionID == "" {
		return QRStart{}, err
	}
	return QRStart{}, errors.Join(err, adapter.CloseQR(context.WithoutCancel(ctx), start.SessionID))
}

func (s *QRService) forget(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func qrTerminal(status string) bool {
	return status == "connected" || status == "expired" || status == "failed"
}
