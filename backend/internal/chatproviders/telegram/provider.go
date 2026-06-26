package telegram

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type Config struct {
	APIID   int
	APIHash string
}

type Store interface {
	ListConnections(context.Context, string) ([]integrations.Connection, error)
	SaveConnection(context.Context, integrations.Connection) error
	SaveIntegrationCursor(context.Context, string, integrations.Cursor) error
}

type RawSink interface {
	WriteRecords(context.Context, []integrations.Record) error
}

type Provider struct {
	cfg    Config
	root   string
	store  Store
	raw    RawSink
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	clients  map[string]clientRun
	sessions map[string]*qrSession

	apiMu      sync.Mutex
	nextAPI    time.Time
	backfillMu sync.Mutex
}

func New(root string, cfg Config, store Store, raw RawSink) (*Provider, error) {
	if cfg.APIID == 0 || strings.TrimSpace(cfg.APIHash) == "" {
		return nil, fmt.Errorf("telegram api id and api hash are required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	return &Provider{
		cfg:      cfg,
		root:     root,
		store:    store,
		raw:      raw,
		ctx:      baseCtx,
		cancel:   cancel,
		clients:  map[string]clientRun{},
		sessions: map[string]*qrSession{},
	}, nil
}

func (p *Provider) ProviderID() string {
	return telegramconnector.ProviderID
}

func (p *Provider) Start(ctx context.Context) error {
	accounts, err := p.store.ListConnections(ctx, telegramconnector.ProviderID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		p.startWatcher(account)
	}
	return nil
}

func (p *Provider) Close() error {
	p.cancel()
	return nil
}
