package cloudsync

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

const sessionExpiredErrorMessage = "Session expired. Sign in again."

type refreshKey struct {
	accountID    string
	refreshToken string
}

type refreshFlight struct {
	done  chan struct{}
	token string
	err   error
}

func isUpstreamUnauthorized(err error) bool {
	var uerr *UpstreamError
	return errors.As(err, &uerr) && uerr.StatusCode == http.StatusUnauthorized
}

func (s *Service) clientFor(accountID string) *cloudClient {
	c := *s.client
	c.refresh = func(ctx context.Context, token string) (string, error) {
		return s.refreshAccountToken(ctx, accountID, token)
	}
	return &c
}

func (s *Service) currentAccountToken(accountID string) (storage.CloudAccount, bool) {
	s.mu.RLock()
	acc, ok := s.accounts[accountID]
	s.mu.RUnlock()
	return acc, ok
}

func (s *Service) latestToken(acc storage.CloudAccount) string {
	if cur, ok := s.currentAccountToken(acc.ID); ok && cur.Token != "" {
		return cur.Token
	}
	return acc.Token
}

func (s *Service) refreshAccountToken(ctx context.Context, accountID, failedToken string) (string, error) {
	s.refreshMu.Lock()
	acc, ok := s.currentAccountToken(accountID)
	if !ok || acc.Token == "" {
		s.refreshMu.Unlock()
		return "", ErrNotConnected
	}
	if acc.Token != failedToken {
		s.refreshMu.Unlock()
		return acc.Token, nil
	}
	if acc.RefreshToken == "" {
		s.refreshMu.Unlock()
		return "", ErrNotConnected
	}
	key := refreshKey{accountID: accountID, refreshToken: acc.RefreshToken}
	if flight := s.refreshFlights[key]; flight != nil {
		s.refreshMu.Unlock()
		select {
		case <-flight.done:
			return flight.token, flight.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	flight := &refreshFlight{done: make(chan struct{})}
	s.refreshFlights[key] = flight
	s.refreshMu.Unlock()

	flight.token, flight.err = s.rotateAccountToken(acc)
	s.refreshMu.Lock()
	delete(s.refreshFlights, key)
	close(flight.done)
	s.refreshMu.Unlock()
	return flight.token, flight.err
}

func (s *Service) rotateAccountToken(previous storage.CloudAccount) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exch, err := s.client.refreshToken(ctx, previous.BaseURL, previous.RefreshToken)
	nextToken := strings.TrimSpace(exch.Token)
	user := exch.User.toUser()
	if err == nil {
		switch {
		case nextToken == "":
			err = errors.New("cloud server did not return a session token")
		case !matchesAccount(previous, user):
			err = errCloudIdentityChanged
		}
	}

	s.persistMu.Lock()
	cur, exists := s.currentAccountToken(previous.ID)
	if !exists || cur.Token == "" {
		s.persistMu.Unlock()
		return "", ErrNotConnected
	}
	if cur.Token != previous.Token || cur.RefreshToken != previous.RefreshToken {
		s.persistMu.Unlock()
		return cur.Token, nil
	}
	switch {
	case isUpstreamUnauthorized(err):
		cur.Token = ""
		cur.RefreshToken = ""
		cur.LastError = sessionExpiredErrorMessage
	case err != nil:
		s.persistMu.Unlock()
		return "", err
	default:
		cur.Token = nextToken
		cur.RefreshToken = cmp.Or(strings.TrimSpace(exch.RefreshToken), cur.RefreshToken)
		cur.LastError = ""
		cur.OrgName = cmp.Or(user.OrgName, cur.OrgName)
		cur.Email = cmp.Or(user.Email, cur.Email)
		cur.DisplayName = cmp.Or(user.DisplayName, cur.DisplayName)
		cur.Role = cmp.Or(user.Role, cur.Role)
	}
	// The upstream may have rotated or revoked credentials even if SQLite is unavailable.
	saveCtx, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	if saveErr := s.store.SaveCloudAccount(saveCtx, cur); saveErr != nil {
		s.log.Warn("save refreshed cloud account failed", "account", cur.ID, "error", saveErr)
	}
	s.mu.Lock()
	s.accounts[cur.ID] = cur
	s.mu.Unlock()
	s.persistMu.Unlock()
	s.notifyAccountChanged(ctx)
	return cur.Token, err
}
