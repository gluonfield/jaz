package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types/events"
)

const whatsappSyncCursorKind = "whatsapp.sync"

func (p *Provider) eventHandler(client *whatsmeow.Client, session *qrSession) whatsmeow.EventHandler {
	return func(evt any) {
		switch event := evt.(type) {
		case *events.PairSuccess:
			p.logInfo("pair success", "session", qrSessionLogID(session), "platform", event.Platform)
			if session != nil {
				session.setAccount(event.ID.User)
				session.setStatus("scanned", "")
			}
		case *events.PairError:
			p.logWarn("pair error", "session", qrSessionLogID(session), "platform", event.Platform, "error", event.Error)
			if session != nil {
				session.setAccount(event.ID.User)
				session.fail(event.Error)
			}
		case *events.Connected:
			connection, ok := connectionFromDevice(client.Store)
			if !ok {
				return
			}
			if session != nil {
				if err := p.store.SaveConnection(p.ctx, connection); err != nil {
					p.logWarn("save connection failed", "session", session.id, "error", err)
					p.failQRSession(p.ctx, session, err)
					return
				}
			}
			p.mu.Lock()
			p.clients[connection.ID] = client
			p.mu.Unlock()
			if session != nil {
				session.setAccount(connection.AccountID)
				session.setStatus("connected", "")
			}
			p.logInfo("client connected", "session", qrSessionLogID(session))
			_ = p.writeAllContacts(p.ctx, connection, client.Store)
			_ = p.writeAllGroups(p.ctx, connection, client)
		case *events.Message:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappMessageRecord(connection, event))
			}
		case *events.HistorySync:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeHistorySync(p.ctx, connection, event.Data, time.Now())
			}
		case *events.Contact:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappContactActionRecord(connection, event))
			}
		case *events.JoinedGroup:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappGroupRecord(connection, event.JID, event.Name))
			}
		case *events.GroupInfo:
			if event.Name == nil {
				return
			}
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappGroupRecord(connection, event.JID, event.Name.Name))
			}
		case *events.LoggedOut:
			p.logWarn("client logged out", "session", qrSessionLogID(session), "reason", event.Reason.String())
			if session != nil {
				session.fail(fmt.Errorf("WhatsApp logged this session out: %s", event.Reason.String()))
			}
		case *events.ClientOutdated:
			p.logWarn("client outdated", "session", qrSessionLogID(session))
			if session != nil {
				session.fail(fmt.Errorf("WhatsApp rejected this client as outdated"))
			}
		case *events.ConnectFailure:
			p.logWarn("connect failure", "session", qrSessionLogID(session), "reason", event.Reason.String())
			if session != nil {
				session.fail(fmt.Errorf("WhatsApp connection failed: %s", event.Reason.String()))
			}
		case *events.TemporaryBan:
			p.logWarn("temporary ban", "session", qrSessionLogID(session), "reason", event.String())
			if session != nil {
				session.fail(fmt.Errorf("WhatsApp temporary ban: %s", event.String()))
			}
		}
	}
}

func qrSessionLogID(session *qrSession) string {
	if session == nil {
		return ""
	}
	return session.id
}

func (p *Provider) writeAllContacts(ctx context.Context, connection integrations.Connection, device *store.Device) error {
	contacts, err := device.Contacts.GetAllContacts(ctx)
	if err != nil {
		return err
	}
	records := make([]integrations.Record, 0, len(contacts))
	for jid, contact := range contacts {
		records = append(records, whatsappContactRecord(connection, jid, contact))
	}
	return p.writeRecords(ctx, records...)
}

func (p *Provider) writeRecords(ctx context.Context, records ...integrations.Record) error {
	if len(records) == 0 || p.raw == nil {
		return nil
	}
	if err := p.raw.WriteRecords(ctx, records); err != nil {
		return err
	}
	return p.store.SaveIntegrationCursor(ctx, records[0].ConnectionID, integrations.Cursor{
		Kind: whatsappSyncCursorKind,
	})
}
