package telegram

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type clientRun struct {
	client *telegram.Client
	cancel context.CancelFunc
	done   <-chan struct{}
}

const clientStopTimeout = 10 * time.Second

func (p *Provider) startWatcher(connection integrations.Connection) {
	p.mu.Lock()
	if p.clients[connection.ID].client != nil {
		p.mu.Unlock()
		return
	}
	dispatcher := p.dispatcherForConnection(connection)
	client := p.newClient(connection.ID, dispatcher, false)
	runCtx, cancel := context.WithCancel(p.ctx)
	done := make(chan struct{})
	p.clients[connection.ID] = clientRun{client: client, cancel: cancel, done: done}
	p.mu.Unlock()

	go func() {
		defer close(done)
		defer p.clearClient(connection.ID, client)
		_ = client.Run(runCtx, func(clientCtx context.Context) error {
			_ = p.writeContacts(clientCtx, connection, client.API())
			p.startBackfillLoop(clientCtx, connection, client.API())
			<-clientCtx.Done()
			return clientCtx.Err()
		})
	}()
}

func (p *Provider) withClient(ctx context.Context, connectionID string, noUpdates bool, fn func(context.Context, *telegram.Client) error) error {
	dispatcher := tg.NewUpdateDispatcher()
	client := p.newClient(connectionID, dispatcher, noUpdates)
	return client.Run(ctx, func(runCtx context.Context) error {
		return fn(runCtx, client)
	})
}

func (p *Provider) newClient(connectionID string, dispatcher tg.UpdateDispatcher, noUpdates bool) *telegram.Client {
	return telegram.NewClient(p.cfg.APIID, p.cfg.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: p.sessionPath(connectionID)},
		UpdateHandler:  dispatcher,
		NoUpdates:      noUpdates,
	})
}

func (p *Provider) sessionPath(connectionID string) string {
	return filepath.Join(p.root, integrations.NormalizeAlias(connectionID)+".json")
}

func (p *Provider) backfillMarkerPath(connectionID string) string {
	return filepath.Join(p.root, integrations.NormalizeAlias(connectionID)+".backfill-complete")
}

func (p *Provider) contactsMarkerPath(connectionID string) string {
	return filepath.Join(p.root, integrations.NormalizeAlias(connectionID)+".contacts-sync")
}

func (p *Provider) backfillComplete(connectionID string) bool {
	_, err := os.Stat(p.backfillMarkerPath(connectionID))
	return err == nil
}

func (p *Provider) setClient(connectionID string, client *telegram.Client, cancel context.CancelFunc, done <-chan struct{}) {
	p.mu.Lock()
	p.clients[connectionID] = clientRun{client: client, cancel: cancel, done: done}
	p.mu.Unlock()
}

func (p *Provider) client(connectionID string) *telegram.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clients[connectionID].client
}

func (p *Provider) clearClient(connectionID string, client *telegram.Client) {
	p.mu.Lock()
	if p.clients[connectionID].client == client {
		delete(p.clients, connectionID)
	}
	p.mu.Unlock()
}

func (p *Provider) removeClient(connectionID string) (*telegram.Client, context.CancelFunc, <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	run := p.clients[connectionID]
	delete(p.clients, connectionID)
	return run.client, run.cancel, run.done
}

func (p *Provider) stopClient(ctx context.Context, connectionID string) error {
	_, cancel, done := p.removeClient(connectionID)
	if cancel != nil {
		cancel()
	}
	return waitClientDone(ctx, done)
}

func waitClientDone(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, clientStopTimeout)
		defer cancel()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
