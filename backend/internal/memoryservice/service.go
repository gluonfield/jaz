// Package memoryservice is the single owner of jaz's embedded memory: the
// jazmem instance, the live enabled gate, the maintenance scheduler, and the
// MCP surface. Everything that consumes memory takes this service instead of
// re-deriving its own gate.
package memoryservice

import (
	"context"
	"net/http"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/gluonfield/jazmem/pkg/jazmemhttp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

type SchedulerControl interface {
	Start()
	Stop()
	Running() bool
}

type Service struct {
	*jazmem.Memory

	Scheduler SchedulerControl

	store  storage.SettingsStorage
	mcpURL string

	mcpOnce sync.Once
	mcp     http.Handler

	mcpServerOnce sync.Once
	mcpServer     *mcpsdk.Server

	apiOnce sync.Once
	api     http.Handler
}

func New(memory *jazmem.Memory, store storage.SettingsStorage, scheduler SchedulerControl, mcpURL string) *Service {
	return &Service{Memory: memory, Scheduler: scheduler, store: store, mcpURL: mcpURL}
}

// Enabled is the live master switch, read per use so toggling needs no restart.
func (s *Service) Enabled() bool {
	return settings.MemoryEnabled(s.store)
}

func (s *Service) MCPURL() string { return s.mcpURL }

func (s *Service) MCPHandler() http.Handler {
	s.mcpOnce.Do(func() { s.mcp = jazmemhttp.NewMCPHandler(s.Memory) })
	return s.mcp
}

func (s *Service) MCPServer() *mcpsdk.Server {
	s.mcpServerOnce.Do(func() { s.mcpServer = jazmemhttp.NewMCPServer(s.Memory) })
	return s.mcpServer
}

// APIHandler serves jazmem's full HTTP API; jaz mounts it under /jazmem so
// the jazmem CLI can target the running server instead of the database file.
func (s *Service) APIHandler() http.Handler {
	s.apiOnce.Do(func() { s.api = jazmemhttp.NewAPIHandler(s.Memory) })
	return s.api
}

// Scheduler runs jazmem's maintenance loop with live start/stop. The
// yaml-level allowed flag stays a hard off switch.
type Scheduler struct {
	memory  *jazmem.Memory
	log     *log.Logger
	allowed bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewScheduler(memory *jazmem.Memory, allowed bool, logger *log.Logger) *Scheduler {
	return &Scheduler{memory: memory, log: logger.WithPrefix("memory"), allowed: allowed}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.allowed || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		if err := s.memory.StartScheduler(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("scheduler stopped", "error", err)
		}
	}()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Scheduler) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil
}
