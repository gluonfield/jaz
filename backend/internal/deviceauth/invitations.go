package deviceauth

import "time"

const PrincipalEnrollment = "enrollment"

type invitation struct {
	owner     Principal
	expiresAt time.Time
}

func (s *Service) CreateInvitation(owner Principal) (string, error) {
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

func (s *Service) invitationOwnerActive(owner Principal) bool {
	if owner.Kind == PrincipalRoot {
		return true
	}
	if owner.Kind != PrincipalDevice {
		return false
	}
	device, err := s.store.LoadDeviceByTokenHash(owner.CredentialHash)
	return err == nil && device.Status == "approved"
}
