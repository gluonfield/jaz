package whatsapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
)

func (p *Provider) startClient(device *store.Device, session *qrSession) {
	client := newWhatsAppClient(device)
	client.AddEventHandler(p.eventHandler(client, session))
	if connection, ok := connectionFromDevice(device); ok {
		p.mu.Lock()
		if p.clients[connection.ID] != nil {
			p.mu.Unlock()
			return
		}
		p.clients[connection.ID] = client
		p.mu.Unlock()
	}
	go func() {
		if err := client.Connect(); err != nil && !errors.Is(err, context.Canceled) {
			if session != nil {
				session.fail(err)
			}
		}
	}()
}

func (p *Provider) clientForConnection(ctx context.Context, connection integrations.Connection) (*whatsmeow.Client, error) {
	p.mu.Lock()
	client := p.clients[connection.ID]
	p.mu.Unlock()
	if client != nil {
		return client, nil
	}
	devices, err := p.container.GetAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	for _, device := range devices {
		candidate, ok := connectionFromDevice(device)
		if !ok || candidate.ID != connection.ID {
			continue
		}
		client = newWhatsAppClient(device)
		client.AddEventHandler(p.eventHandler(client, nil))
		p.mu.Lock()
		p.clients[connection.ID] = client
		p.mu.Unlock()
		return client, nil
	}
	return nil, fmt.Errorf("whatsapp session not found for connection %s", connection.ID)
}

func (p *Provider) Disconnect(ctx context.Context, connection integrations.Connection) error {
	client := p.removeClient(connection.ID)
	if client != nil {
		client.RemoveEventHandlers()
		if err := client.Logout(ctx); err == nil {
			return nil
		}
		client.Disconnect()
		return client.Store.Delete(ctx)
	}
	device, ok, err := p.deviceForConnection(ctx, connection.ID)
	if err != nil || !ok {
		return err
	}
	return device.Delete(ctx)
}

func (p *Provider) removeClient(connectionID string) *whatsmeow.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	client := p.clients[connectionID]
	delete(p.clients, connectionID)
	return client
}

func (p *Provider) deviceForConnection(ctx context.Context, connectionID string) (*store.Device, bool, error) {
	devices, err := p.container.GetAllDevices(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, device := range devices {
		connection, ok := connectionFromDevice(device)
		if ok && connection.ID == connectionID {
			return device, true, nil
		}
	}
	return nil, false, nil
}
