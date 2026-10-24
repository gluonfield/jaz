package deviceauth

import (
	"context"
	"sync"
	"time"
)

type access struct {
	mu          sync.Mutex
	active      map[string]map[context.Context]context.CancelFunc
	tickets     map[string]ticketAccess
	invitations map[string]invitation
}

func (s *Service) Track(ctx context.Context, principal Principal) (context.Context, func(), error) {
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	device, err := s.store.LoadDeviceByTokenHash(principal.CredentialHash)
	if err != nil || device.Status != "approved" {
		return nil, nil, ErrUnauthorized
	}
	deviceID := device.ID
	ctx, cancel := context.WithCancel(ctx)
	if s.access.active == nil {
		s.access.active = make(map[string]map[context.Context]context.CancelFunc)
	}
	if s.access.active[deviceID] == nil {
		s.access.active[deviceID] = make(map[context.Context]context.CancelFunc)
	}
	s.access.active[deviceID][ctx] = cancel
	return ctx, func() {
		cancel()
		s.access.mu.Lock()
		defer s.access.mu.Unlock()
		delete(s.access.active[deviceID], ctx)
		if len(s.access.active[deviceID]) == 0 {
			delete(s.access.active, deviceID)
		}
	}, nil
}

type ResourceTicket struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ticketAccess struct {
	tokenHash string
	expiresAt time.Time
}

func (s *Service) IssueResourceTicket(credential string) (ResourceTicket, error) {
	token, err := newToken()
	if err != nil {
		return ResourceTicket{}, err
	}
	ticket := ResourceTicket{Token: token, ExpiresAt: s.now().Add(time.Minute)}
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	if s.access.tickets == nil {
		s.access.tickets = make(map[string]ticketAccess)
	}
	for key, entry := range s.access.tickets {
		if !s.now().Before(entry.expiresAt) {
			delete(s.access.tickets, key)
		}
	}
	device, err := s.store.LoadDeviceByTokenHash(TokenHash(credential))
	if err != nil || device.Status != "approved" {
		return ResourceTicket{}, ErrUnauthorized
	}
	s.access.tickets[TokenHash(token)] = ticketAccess{tokenHash: TokenHash(credential), expiresAt: ticket.ExpiresAt}
	return ticket, nil
}

func (s *Service) AuthenticateResourceTicket(token string) (Principal, error) {
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	entry, ok := s.access.tickets[TokenHash(token)]
	if !ok || !s.now().Before(entry.expiresAt) {
		return Principal{}, ErrUnauthorized
	}
	device, err := s.store.LoadDeviceByTokenHash(entry.tokenHash)
	if err != nil || device.Status != "approved" {
		return Principal{}, ErrUnauthorized
	}
	return Principal{Kind: PrincipalDevice, DeviceID: device.ID, CredentialHash: entry.tokenHash}, nil
}

func (s *Service) cancelAccessLocked(deviceID string) {
	for _, cancel := range s.access.active[deviceID] {
		cancel()
	}
}
