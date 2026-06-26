package whatsapp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

type Store interface {
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	SaveConnection(context.Context, integrations.Connection) error
}

type RawSink interface {
	WriteRecords(context.Context, []integrations.Record) error
}

type Provider struct {
	store     Store
	raw       RawSink
	container *sqlstore.Container
	ctx       context.Context
	cancel    context.CancelFunc

	mu       sync.Mutex
	clients  map[string]*whatsmeow.Client
	sessions map[string]*qrSession
}

func New(ctx context.Context, root string, store Store, raw RawSink) (*Provider, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "whatsapp.sqlite"))
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		_ = container.Close()
		return nil, err
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	return &Provider{
		store:     store,
		raw:       raw,
		container: container,
		ctx:       baseCtx,
		cancel:    cancel,
		clients:   map[string]*whatsmeow.Client{},
		sessions:  map[string]*qrSession{},
	}, nil
}

func (p *Provider) ProviderID() string {
	return whatsappconnector.ProviderID
}

func (p *Provider) Start(ctx context.Context) error {
	accounts, err := p.store.ListConnections(ctx, whatsappconnector.ProviderID)
	if err != nil {
		return err
	}
	allowed := connectionIDs(accounts)
	devices, err := p.container.GetAllDevices(ctx)
	if err != nil {
		return err
	}
	for _, device := range devices {
		connection, ok := connectionFromDevice(device)
		if !ok {
			continue
		}
		if !allowed[connection.ID] {
			if err := device.Delete(ctx); err != nil {
				return err
			}
			continue
		}
		p.startClient(device, nil)
	}
	return nil
}

func (p *Provider) Close() error {
	p.cancel()
	p.mu.Lock()
	clients := make([]*whatsmeow.Client, 0, len(p.clients))
	for _, client := range p.clients {
		clients = append(clients, client)
	}
	p.mu.Unlock()
	for _, client := range clients {
		client.Disconnect()
	}
	return p.container.Close()
}

func connectionIDs(connections []integrations.Connection) map[string]bool {
	out := map[string]bool{}
	for _, connection := range connections {
		if connection.ID != "" {
			out[connection.ID] = true
		}
	}
	return out
}
