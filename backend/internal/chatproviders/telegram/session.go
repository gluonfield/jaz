package telegram

import (
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/connections"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type qrSession struct {
	mu           sync.Mutex
	id           string
	connectionID string
	code         string
	status       string
	expiresAt    time.Time
	accountID    string
	connection   *integrations.Connection
	err          string
	ready        chan struct{}
	readyOnce    sync.Once
}

func (s *qrSession) setCode(code string, expiresAt time.Time) {
	s.mu.Lock()
	s.code = code
	s.expiresAt = expiresAt
	if s.status == "" {
		s.status = "pending"
	}
	s.mu.Unlock()
	s.readyOnce.Do(func() { close(s.ready) })
}

func (s *qrSession) setConnection(connection integrations.Connection) {
	s.mu.Lock()
	s.connection = &connection
	s.accountID = connection.AccountID
	s.mu.Unlock()
}

func (s *qrSession) connectionSnapshot() (integrations.Connection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connection == nil {
		return integrations.Connection{}, false
	}
	return *s.connection, true
}

func (s *qrSession) setStatus(status, err string) {
	s.mu.Lock()
	s.status = status
	s.err = err
	s.mu.Unlock()
}

func (s *qrSession) fail(err error) {
	if err == nil {
		return
	}
	s.setStatus("failed", err.Error())
	s.readyOnce.Do(func() { close(s.ready) })
}

func (s *qrSession) statusSnapshot() connections.QRStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	if status == "" {
		status = "pending"
	}
	if status == "pending" && !s.expiresAt.IsZero() && time.Now().After(s.expiresAt) {
		status = "expired"
	}
	return connections.QRStatus{
		SessionID: s.id,
		Provider:  telegramconnector.ProviderID,
		Code:      s.code,
		Status:    status,
		ExpiresAt: s.expiresAt,
		AccountID: s.accountID,
		Error:     s.err,
	}
}
