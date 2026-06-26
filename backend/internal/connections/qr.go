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
	Instructions []string  `json:"instructions,omitempty"`
}

type QRStatus struct {
	SessionID string    `json:"session_id"`
	Provider  string    `json:"provider"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	AccountID string    `json:"account_id,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type QRProvider interface {
	ProviderID() string
	StartQR(context.Context) (QRStart, error)
	QRStatus(context.Context, string) (QRStatus, error)
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
		return QRStart{}, fmt.Errorf("connection QR provider %q returned an incomplete session", provider)
	}
	if start.Provider == "" {
		start.Provider = provider
	}
	if start.Provider != provider {
		return QRStart{}, fmt.Errorf("connection QR provider %q returned provider %q", provider, start.Provider)
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
		return QRStatus{}, err
	}
	if status.SessionID == "" {
		status.SessionID = id
	}
	if status.Provider == "" {
		status.Provider = provider
	}
	return status, nil
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
