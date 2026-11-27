package cloudsync

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

const sessionExpiredErrorMessage = "Session expired. Sign in again."

type refreshFlight struct {
	done chan struct{}
	acc  storage.CloudAccount
	err  error
}

func isUpstreamUnauthorized(err error) bool {
	var uerr *UpstreamError
	return errors.As(err, &uerr) && uerr.StatusCode == http.StatusUnauthorized
}

func (s *Service) clientFor(accountID string) *cloudClient {
	c := *s.client
	c.accountID = accountID
	c.refresh = s.refreshAccountToken
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
	refreshed, err := s.refreshAccount(ctx, accountID, failedToken)
	if err != nil {
		return "", err
	}
	return refreshed.Token, nil
}

func (s *Service) refreshAccount(ctx context.Context, accountID, failedToken string) (storage.CloudAccount, error) {
	s.refreshMu.Lock()
	if flight := s.refreshFlights[accountID]; flight != nil {
		s.refreshMu.Unlock()
		select {
		case <-flight.done:
			return flight.acc, flight.err
		case <-ctx.Done():
			return storage.CloudAccount{}, ctx.Err()
		}
	}
	acc, ok := s.currentAccountToken(accountID)
	if !ok || acc.Token == "" {
		s.refreshMu.Unlock()
		return storage.CloudAccount{}, ErrNotConnected
	}
	if acc.Token != failedToken {
		s.refreshMu.Unlock()
		return acc, nil
	}
	if acc.RefreshToken == "" {
		s.refreshMu.Unlock()
		return storage.CloudAccount{}, &UpstreamError{StatusCode: http.StatusUnauthorized, Message: ErrNotConnected.Error()}
	}
	flight := &refreshFlight{done: make(chan struct{})}
	s.refreshFlights[accountID] = flight
	s.refreshMu.Unlock()

	defer func() {
		s.refreshMu.Lock()
		delete(s.refreshFlights, accountID)
		s.refreshMu.Unlock()
		close(flight.done)
	}()

	refreshCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exch, err := s.client.refreshToken(refreshCtx, acc.BaseURL, acc.RefreshToken)
	if err != nil {
		if isUpstreamUnauthorized(err) {
			s.clearExpiredAccountTokens(accountID, acc.RefreshToken)
		}
		flight.err = err
		return storage.CloudAccount{}, err
	}
	nextToken := strings.TrimSpace(exch.Token)
	if nextToken == "" {
		flight.err = errors.New("cloud server did not return a session token")
		return storage.CloudAccount{}, flight.err
	}
	nextRefresh := strings.TrimSpace(exch.RefreshToken)
	if nextRefresh == "" {
		nextRefresh = acc.RefreshToken
	}

	user := exch.User.toUser()
	var (
		committed storage.CloudAccount
		changed   bool
	)
	s.persistMu.Lock()
	cur, exists := s.currentAccountToken(accountID)
	if exists && cur.Token == failedToken && cur.RefreshToken == acc.RefreshToken {
		updated := cur
		updated.Token = nextToken
		updated.RefreshToken = nextRefresh
		updated.LastError = ""
		if user.OrgName != "" {
			updated.OrgName = user.OrgName
		}
		if user.Email != "" {
			updated.Email = user.Email
		}
		if user.DisplayName != "" {
			updated.DisplayName = user.DisplayName
		}
		if user.Role != "" {
			updated.Role = user.Role
		}
		if err := s.store.SaveCloudAccount(refreshCtx, updated); err != nil {
			s.log.Warn("save refreshed cloud account failed", "account", accountID, "error", err)
		}
		s.mu.Lock()
		s.accounts[accountID] = updated
		s.mu.Unlock()
		committed = updated
		changed = true
	} else if exists && cur.Token != "" {
		committed = cur
	}
	s.persistMu.Unlock()

	if changed {
		s.notifyAccountChanged(context.Background())
		flight.acc = committed
		return committed, nil
	}
	if committed.Token != "" {
		flight.acc = committed
		return committed, nil
	}
	flight.err = ErrNotConnected
	return storage.CloudAccount{}, ErrNotConnected
}

func (s *Service) clearExpiredAccountTokens(accountID, expiredRefresh string) {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cleared bool
	s.persistMu.Lock()
	cur, ok := s.currentAccountToken(accountID)
	if ok && cur.RefreshToken == expiredRefresh {
		updated := cur
		updated.Token = ""
		updated.RefreshToken = ""
		updated.LastError = sessionExpiredErrorMessage
		if err := s.store.SaveCloudAccount(saveCtx, updated); err != nil {
			s.log.Warn("clear expired cloud account tokens failed", "account", accountID, "error", err)
		}
		s.mu.Lock()
		s.accounts[accountID] = updated
		s.mu.Unlock()
		cleared = true
	}
	s.persistMu.Unlock()

	if cleared {
		s.notifyAccountChanged(context.Background())
	}
}

func (s *Service) doAccountHTTP(ctx context.Context, acc storage.CloudAccount, open func(token string) (*http.Response, error)) (*http.Response, error) {
	tok := s.latestToken(acc)
	resp, err := open(tok)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	refreshed, rerr := s.refreshAccount(ctx, acc.ID, tok)
	if rerr != nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	return open(refreshed.Token)
}
