package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wins/jaz/backend/internal/connections"
	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"go.mau.fi/whatsmeow"
)

const firstQRCodeTimeout = 20 * time.Second

func (p *Provider) StartQR(ctx context.Context) (connections.QRStart, error) {
	refreshWhatsAppWebVersion(ctx)
	device := p.container.NewDevice()
	client := newWhatsAppClient(device)
	session := &qrSession{
		id:        "whatsapp_qr_" + uuid.NewString(),
		status:    "pending",
		ready:     make(chan struct{}),
		client:    client,
		expiresAt: time.Now().UTC().Add(time.Minute),
	}
	p.mu.Lock()
	p.sessions[session.id] = session
	p.mu.Unlock()

	qrChan, err := client.GetQRChannel(p.ctx)
	if err != nil {
		_ = p.CloseQR(context.WithoutCancel(ctx), session.id)
		return connections.QRStart{}, err
	}
	client.AddEventHandler(p.eventHandler(client, session))
	go p.watchQR(session, qrChan)
	go func() {
		if err := client.Connect(); err != nil && !errors.Is(err, context.Canceled) {
			p.failQRSession(p.ctx, session, err)
		}
	}()

	timer := time.NewTimer(firstQRCodeTimeout)
	defer timer.Stop()
	select {
	case <-session.ready:
		status := session.statusSnapshot()
		if status.Code == "" {
			_ = p.CloseQR(context.WithoutCancel(ctx), session.id)
			return connections.QRStart{}, whatsappFirstQRCodeError(status)
		}
		return connections.QRStart{
			SessionID: session.id,
			Provider:  whatsappconnector.ProviderID,
			Code:      status.Code,
			Status:    status.Status,
			ExpiresAt: status.ExpiresAt,
			Instructions: []string{
				"Open WhatsApp on your phone.",
				"Go to Linked devices.",
				"Scan this QR code.",
			},
		}, nil
	case <-timer.C:
		err := fmt.Errorf("timed out waiting for WhatsApp to return a QR code")
		session.fail(err)
		if removed := p.removeQRSession(session.id); removed != nil {
			p.teardownQRSession(context.WithoutCancel(ctx), removed)
		}
		return connections.QRStart{}, err
	case <-ctx.Done():
		_ = p.CloseQR(context.WithoutCancel(ctx), session.id)
		return connections.QRStart{}, ctx.Err()
	}
}

func whatsappFirstQRCodeError(status connections.QRStatus) error {
	if status.Error != "" {
		return fmt.Errorf("WhatsApp QR sign-in failed: %s", status.Error)
	}
	if status.Status == "expired" {
		return fmt.Errorf("WhatsApp QR code expired before it was shown")
	}
	return fmt.Errorf("WhatsApp did not return a QR code")
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
		p.finishQRSession(context.WithoutCancel(ctx), id, session)
	}
	return status, nil
}

func (p *Provider) CloseQR(ctx context.Context, id string) error {
	session := p.removeQRSession(id)
	if session == nil {
		return connections.ErrQRSessionNotFound
	}
	p.teardownQRSession(context.WithoutCancel(ctx), session)
	return nil
}

func (p *Provider) failQRSession(ctx context.Context, session *qrSession, err error) {
	session.fail(err)
	p.teardownQRSession(context.WithoutCancel(ctx), session)
}

func (p *Provider) finishQRSession(ctx context.Context, id string, session *qrSession) {
	p.forgetQRSession(id, session)
	p.teardownQRSession(ctx, session)
}

func (p *Provider) removeQRSession(id string) *qrSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[id]
	if session != nil {
		delete(p.sessions, id)
	}
	return session
}

func (p *Provider) forgetQRSession(id string, session *qrSession) {
	p.mu.Lock()
	if p.sessions[id] == session {
		delete(p.sessions, id)
	}
	p.mu.Unlock()
}

func (p *Provider) teardownQRSession(ctx context.Context, session *qrSession) {
	if session.statusSnapshot().Status == "connected" {
		return
	}
	if session.client == nil {
		return
	}
	session.client.RemoveEventHandlers()
	session.client.Disconnect()
	if session.client.Store != nil && session.client.Store.ID != nil {
		_ = session.client.Store.Delete(ctx)
	}
}

func qrDone(status string) bool {
	return status == "connected" || status == "expired" || status == "failed"
}

func (p *Provider) watchQR(session *qrSession, qrChan <-chan whatsmeow.QRChannelItem) {
	for item := range qrChan {
		switch item.Event {
		case whatsmeow.QRChannelEventCode:
			session.setCode(item.Code, time.Now().UTC().Add(item.Timeout))
		case whatsmeow.QRChannelSuccess.Event:
			session.setStatus("scanned", "")
		case whatsmeow.QRChannelTimeout.Event:
			session.setStatus("expired", "")
			session.readyOnce.Do(func() { close(session.ready) })
			p.teardownQRSession(p.ctx, session)
			return
		case whatsmeow.QRChannelClientOutdated.Event:
			session.setStatus("failed", "WhatsApp rejected this client as outdated")
			session.readyOnce.Do(func() { close(session.ready) })
			p.teardownQRSession(p.ctx, session)
			return
		case whatsmeow.QRChannelScannedWithoutMultidevice.Event:
			session.setStatus("pending", "Enable linked-device support in WhatsApp, then scan the same code again")
		case whatsmeow.QRChannelEventError:
			if item.Error != nil {
				p.failQRSession(p.ctx, session, item.Error)
				return
			}
			session.setStatus("failed", item.Event)
			session.readyOnce.Do(func() { close(session.ready) })
			p.teardownQRSession(p.ctx, session)
			return
		default:
			session.setStatus("failed", item.Event)
			session.readyOnce.Do(func() { close(session.ready) })
			p.teardownQRSession(p.ctx, session)
			return
		}
	}
}
