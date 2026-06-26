package whatsapp

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types/events"
)

func (p *Provider) eventHandler(client *whatsmeow.Client, session *qrSession) whatsmeow.EventHandler {
	return func(evt any) {
		switch event := evt.(type) {
		case *events.PairSuccess:
			if session != nil {
				session.setAccount(event.ID.User)
				session.setStatus("scanned", "")
			}
		case *events.Connected:
			connection, ok := connectionFromDevice(client.Store)
			if !ok {
				return
			}
			if session != nil {
				if err := p.store.SaveConnection(p.ctx, connection); err != nil {
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
			_ = p.writeAllContacts(p.ctx, connection, client.Store)
		case *events.Message:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappMessageRecord(connection, event))
			}
		case *events.HistorySync:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappHistoryRecords(connection, event.Data, whatsappHistoryCutoff(time.Now()))...)
			}
		case *events.Contact:
			if connection, ok := connectionFromDevice(client.Store); ok {
				_ = p.writeRecords(p.ctx, whatsappContactActionRecord(connection, event))
			}
		case *events.LoggedOut:
			if session != nil {
				session.setStatus("failed", event.Reason.String())
			}
		}
	}
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
	return p.raw.WriteRecords(ctx, records)
}
