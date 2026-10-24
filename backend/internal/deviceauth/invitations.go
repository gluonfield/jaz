package deviceauth

import (
	"strings"
	"time"
)

const PrincipalEnrollment = "enrollment"

type invitation struct {
	owner     string
	expiresAt time.Time
}

func (s *Service) CreateInvitation(owner string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	token = "invite_" + token
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	if !s.invitationOwnerActive(owner) {
		return "", ErrUnauthorized
	}
	if s.access.invitations == nil {
		s.access.invitations = make(map[string]invitation)
	}
	for hash, entry := range s.access.invitations {
		if !s.now().Before(entry.expiresAt) {
			delete(s.access.invitations, hash)
		}
	}
	s.access.invitations[TokenHash(token)] = invitation{owner: owner, expiresAt: s.now().Add(pairingTTL)}
	return token, nil
}

func (s *Service) IsInvitation(token string) bool {
	if !strings.HasPrefix(token, "invite_") {
		return false
	}
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	entry, ok := s.access.invitations[TokenHash(token)]
	return ok && s.now().Before(entry.expiresAt) && s.invitationOwnerActive(entry.owner)
}

func (s *Service) RegisterInvited(token string, reg Registration) (RegisterResult, error) {
	reg, err := normalizeRegistration(reg)
	if err != nil {
		return RegisterResult{}, err
	}
	s.access.mu.Lock()
	defer s.access.mu.Unlock()
	entry, ok := s.access.invitations[TokenHash(token)]
	if !ok || !s.now().Before(entry.expiresAt) || !s.invitationOwnerActive(entry.owner) {
		return RegisterResult{}, ErrUnauthorized
	}
	result, err := s.saveApprovedDevice(reg)
	if err == nil {
		delete(s.access.invitations, TokenHash(token))
	}
	return result, err
}

func (s *Service) invitationOwnerActive(owner string) bool {
	if owner == "" {
		return true
	}
	device, err := s.store.LoadDevice(owner)
	return err == nil && device.Status == "approved"
}
