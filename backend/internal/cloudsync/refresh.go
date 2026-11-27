package cloudsync

import (
	"context"
	"encoding/json"
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

func (s *Service) currentAccountToken(accountID string) (storage.CloudAccount, bool) {
	s.mu.RLock()
	acc, ok := s.accounts[accountID]
	s.mu.RUnlock()
	return acc, ok
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

	s.mu.RLock()
	cur, exists := s.accounts[accountID]
	s.mu.RUnlock()
	if !exists || cur.Token != failedToken || cur.RefreshToken != acc.RefreshToken {
		if exists && cur.Token != "" {
			flight.acc = cur
			return cur, nil
		}
		flight.err = ErrNotConnected
		return storage.CloudAccount{}, ErrNotConnected
	}

	updated := cur
	updated.Token = nextToken
	updated.RefreshToken = nextRefresh
	updated.LastError = ""
	user := exch.User.toUser()
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
	if latest, ok := s.accounts[accountID]; ok && latest.Token == failedToken && latest.RefreshToken == acc.RefreshToken {
		s.accounts[accountID] = updated
		s.mu.Unlock()
		s.notifyAccountChanged(context.Background())
		flight.acc = updated
		return updated, nil
	}
	latest, ok := s.accounts[accountID]
	s.mu.Unlock()
	if ok && latest.Token != "" {
		flight.acc = latest
		return latest, nil
	}
	flight.err = ErrNotConnected
	return storage.CloudAccount{}, ErrNotConnected
}

func (s *Service) clearExpiredAccountTokens(accountID, expiredRefresh string) {
	s.mu.RLock()
	cur, ok := s.accounts[accountID]
	s.mu.RUnlock()
	if !ok || cur.RefreshToken != expiredRefresh {
		return
	}
	cleared := cur
	cleared.Token = ""
	cleared.RefreshToken = ""
	cleared.LastError = sessionExpiredErrorMessage

	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.SaveCloudAccount(saveCtx, cleared); err != nil {
		s.log.Warn("clear expired cloud account tokens failed", "account", accountID, "error", err)
	}

	s.mu.Lock()
	if latest, exists := s.accounts[accountID]; exists && latest.RefreshToken == expiredRefresh {
		s.accounts[accountID] = cleared
		s.mu.Unlock()
		s.notifyAccountChanged(context.Background())
		return
	}
	s.mu.Unlock()
}

func withAccountRefresh[T any](ctx context.Context, s *Service, acc storage.CloudAccount, fn func(token string) (T, error)) (T, error) {
	res, err := fn(acc.Token)
	if !isUpstreamUnauthorized(err) {
		return res, err
	}
	refreshed, rerr := s.refreshAccount(ctx, acc.ID, acc.Token)
	if rerr != nil {
		return res, err
	}
	return fn(refreshed.Token)
}

func (s *Service) doAccountJSON(ctx context.Context, acc storage.CloudAccount, method, path string, body any) (json.RawMessage, int, error) {
	raw, status, err := s.client.doJSON(ctx, method, acc.BaseURL, path, acc.Token, body)
	if !isUpstreamUnauthorized(err) {
		return raw, status, err
	}
	refreshed, rerr := s.refreshAccount(ctx, acc.ID, acc.Token)
	if rerr != nil {
		return raw, status, err
	}
	return s.client.doJSON(ctx, method, refreshed.BaseURL, path, refreshed.Token, body)
}

func (s *Service) doAccountHTTP(ctx context.Context, acc storage.CloudAccount, open func(token string) (*http.Response, error)) (*http.Response, error) {
	resp, err := open(acc.Token)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	refreshed, rerr := s.refreshAccount(ctx, acc.ID, acc.Token)
	if rerr != nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	return open(refreshed.Token)
}
