package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/wins/jaz/backend/internal/connections"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
)

const firstQRCodeTimeout = 20 * time.Second

func (p *Provider) StartQR(ctx context.Context) (connections.QRStart, error) {
	connectionID := telegramconnector.ProviderID + ":" + uuid.NewString()
	loginCtx, cancel := context.WithCancel(p.ctx)
	session := &qrSession{
		id:           "telegram_qr_" + uuid.NewString(),
		connectionID: connectionID,
		status:       "pending",
		ready:        make(chan struct{}),
		expiresAt:    time.Now().UTC().Add(time.Minute),
	}
	p.mu.Lock()
	p.sessions[session.id] = session
	p.mu.Unlock()

	dispatcher := p.dispatcherForSession(session)
	loggedIn := qrlogin.OnLoginToken(&dispatcher)
	client := p.newClient(connectionID, dispatcher, false)
	done := make(chan struct{})
	p.setClient(connectionID, client, cancel, done)

	go func() {
		defer close(done)
		err := client.Run(loginCtx, func(runCtx context.Context) error {
			_, err := client.QR().Auth(runCtx, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
				session.setCode(token.URL(), token.Expires())
				return nil
			})
			if err != nil {
				session.fail(err)
				return nil
			}
			self, err := client.Self(runCtx)
			if err != nil {
				session.fail(err)
				return nil
			}
			connection := telegramConnection(connectionID, self)
			if err := p.store.SaveConnection(runCtx, connection); err != nil {
				session.fail(err)
				return nil
			}
			session.setConnection(connection)
			session.setStatus("connected", "")
			_ = p.writeContacts(runCtx, connection, client.API())
			p.startBackfillLoop(runCtx, connection, client.API())
			<-runCtx.Done()
			return runCtx.Err()
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			session.fail(err)
		}
		if session.statusSnapshot().Status == "failed" {
			_ = removeFile(p.sessionPath(connectionID))
		}
		p.clearClient(connectionID, client)
	}()

	timer := time.NewTimer(firstQRCodeTimeout)
	defer timer.Stop()
	select {
	case <-session.ready:
		status := session.statusSnapshot()
		if status.Code == "" {
			_ = p.CloseQR(context.WithoutCancel(ctx), session.id)
			return connections.QRStart{}, fmt.Errorf("telegram QR provider did not return a code")
		}
		return connections.QRStart{
			SessionID: session.id,
			Provider:  telegramconnector.ProviderID,
			Code:      status.Code,
			Status:    status.Status,
			ExpiresAt: status.ExpiresAt,
			Instructions: []string{
				"Open Telegram on your phone.",
				"Go to Settings, then Devices.",
				"Scan this QR code.",
			},
		}, nil
	case <-timer.C:
		err := fmt.Errorf("timed out waiting for Telegram to return a QR code")
		session.fail(err)
		_ = p.closeQRSession(context.WithoutCancel(ctx), session)
		return connections.QRStart{}, err
	case <-ctx.Done():
		_ = p.closeQRSession(context.WithoutCancel(ctx), session)
		return connections.QRStart{}, ctx.Err()
	}
}

func (p *Provider) QRStatus(ctx context.Context, id string) (connections.QRStatus, error) {
	p.mu.Lock()
	session := p.sessions[id]
	p.mu.Unlock()
	if session == nil {
		return connections.QRStatus{}, connections.ErrQRSessionNotFound
	}
	status := session.statusSnapshot()
	if qrDone(status.Status) {
		_ = p.CloseQR(context.WithoutCancel(ctx), id)
	}
	return status, nil
}

func (p *Provider) CloseQR(ctx context.Context, id string) error {
	p.mu.Lock()
	session := p.sessions[id]
	if session != nil {
		delete(p.sessions, id)
	}
	p.mu.Unlock()
	if session == nil {
		return connections.ErrQRSessionNotFound
	}
	return p.closeQRSession(ctx, session)
}

func (p *Provider) closeQRSession(ctx context.Context, session *qrSession) error {
	p.clearSession(session.id, session)
	if session.statusSnapshot().Status == "connected" {
		return nil
	}
	return errors.Join(
		p.stopClient(ctx, session.connectionID),
		removeFile(p.sessionPath(session.connectionID)),
	)
}

func (p *Provider) clearSession(id string, session *qrSession) {
	p.mu.Lock()
	if p.sessions[id] == session {
		delete(p.sessions, id)
	}
	p.mu.Unlock()
}

func qrDone(status string) bool {
	return status == "connected" || status == "expired" || status == "failed"
}
