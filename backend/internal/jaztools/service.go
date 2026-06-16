package jaztools

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/loops"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/visualize"
)

const (
	ServerID   = "jaztools"
	ServerName = "jaztools"
	Version    = "0.1.0"
)

func ServerConfig(url string) mcpconfig.Server {
	return mcpconfig.Server{
		ID:        ServerID,
		Name:      ServerName,
		Transport: mcpconfig.TransportStreamableHTTP,
		URL:       strings.TrimSpace(url),
		Enabled:   true,
		Headers: []mcpconfig.Header{{
			Name:  mcpsession.HeaderName,
			Value: mcpsession.HeaderPlaceholder,
		}},
	}
}

type Service struct {
	Memory *memoryservice.Service

	loopTools      *loops.MCPTools
	visualizeTools *visualize.MCPTools

	url string

	mu          sync.Mutex
	memoryTools bool
	serverOnce  sync.Once
	server      *mcp.Server
	handlerOnce sync.Once
	handler     http.Handler
}

func New(memory *memoryservice.Service, urls serverconfig.URLs, store *sqlitestore.Store, events *sessionevents.Bus) *Service {
	return &Service{
		Memory:         memory,
		visualizeTools: visualize.NewMCPTools(store, events),
		url:            strings.TrimSpace(urls.JazToolsMCP),
	}
}

func (s *Service) URL() string {
	return s.url
}

func (s *Service) SetLoops(service loops.MCPService) {
	s.loopTools = loops.NewMCPTools(service)
}

func (s *Service) Server() *mcp.Server {
	s.serverOnce.Do(func() {
		server := mcp.NewServer(&mcp.Implementation{
			Name:    ServerName,
			Title:   "Jaz Tools",
			Version: Version,
		}, nil)
		s.loopTools.AddTo(server)
		s.visualizeTools.AddTo(server)
		s.mu.Lock()
		s.server = server
		s.syncMemoryTools()
		s.mu.Unlock()
	})
	return s.server
}

func (s *Service) Sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncMemoryTools()
}

func (s *Service) Handler() http.Handler {
	s.handlerOnce.Do(func() {
		s.handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return s.Server()
		}, &mcp.StreamableHTTPOptions{
			JSONResponse:   true,
			SessionTimeout: 30 * time.Minute,
		})
	})
	return s.handler
}

func (s *Service) syncMemoryTools() {
	if s.server == nil {
		return
	}
	if s.Memory.MCPToolsEnabled() {
		if !s.memoryTools {
			s.Memory.AddMCPTools(s.server)
			s.memoryTools = true
		}
		return
	}
	if s.memoryTools {
		s.Memory.RemoveMCPTools(s.server)
		s.memoryTools = false
	}
}
