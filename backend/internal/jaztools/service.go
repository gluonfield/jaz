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
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
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

	loopTools       *loops.MCPTools
	visualizeTools  *visualize.MCPTools
	widgetPublisher widgets.MCPPublisher
	sessions        storage.SessionStore

	url string

	mu          sync.Mutex
	thread      serverSlot
	widget      serverSlot
	handlerOnce sync.Once
	handler     http.Handler
}

type toolSurface int

const (
	threadSurface toolSurface = iota
	widgetSurface
)

type serverSlot struct {
	once        sync.Once
	server      *mcp.Server
	memoryTools bool
}

func New(memory *memoryservice.Service, urls serverconfig.URLs, sessions storage.SessionStore, sessionEvents storage.SessionEventAppender, events *sessionevents.Bus, widgetPublisher *widgets.SessionPublisher) *Service {
	return &Service{
		Memory:          memory,
		visualizeTools:  visualize.NewMCPTools(sessionEvents, events),
		widgetPublisher: widgetPublisher,
		sessions:        sessions,
		url:             strings.TrimSpace(urls.JazToolsMCP),
	}
}

func (s *Service) URL() string {
	return s.url
}

func (s *Service) SetLoops(service loops.MCPService) {
	s.loopTools = loops.NewMCPTools(service)
}

func (s *Service) Server() *mcp.Server {
	return s.server(threadSurface)
}

func (s *Service) server(surface toolSurface) *mcp.Server {
	slot := s.slot(surface)
	slot.once.Do(func() {
		slot.server = s.newServer(surface)
		s.Sync()
	})
	return slot.server
}

func (s *Service) slot(surface toolSurface) *serverSlot {
	switch surface {
	case widgetSurface:
		return &s.widget
	default:
		return &s.thread
	}
}

func (s *Service) newServer(surface toolSurface) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   "Jaz Tools",
		Version: Version,
	}, nil)
	s.loopTools.AddTo(server)
	s.visualizeTools.AddReadMeTo(server)
	switch surface {
	case widgetSurface:
		widgets.AddMCPTools(server, s.widgetPublisher)
	default:
		s.visualizeTools.AddShowWidgetTo(server)
	}
	return server
}

func (s *Service) Sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncMemoryTools()
}

func (s *Service) Handler() http.Handler {
	s.handlerOnce.Do(func() {
		s.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s.server(s.surface(r))
		}, &mcp.StreamableHTTPOptions{
			JSONResponse:   true,
			SessionTimeout: 30 * time.Minute,
		})
	})
	return s.handler
}

func (s *Service) surface(r *http.Request) toolSurface {
	if s.widgetSession(r) {
		return widgetSurface
	}
	return threadSurface
}

func (s *Service) widgetSession(r *http.Request) bool {
	sessionID := strings.TrimSpace(r.Header.Get(mcpsession.HeaderName))
	if sessionID == "" {
		return false
	}
	session, err := s.sessions.LoadSession(sessionID)
	return err == nil && session.SourceType == storage.SourceLoopRun
}

func (s *Service) syncMemoryTools() {
	s.syncMemoryToolsFor(&s.thread)
	s.syncMemoryToolsFor(&s.widget)
}

func (s *Service) syncMemoryToolsFor(slot *serverSlot) {
	if slot.server == nil {
		return
	}
	if s.Memory.MCPToolsEnabled() {
		if !slot.memoryTools {
			s.Memory.AddMCPTools(slot.server)
			slot.memoryTools = true
		}
		return
	}
	if slot.memoryTools {
		s.Memory.RemoveMCPTools(slot.server)
		slot.memoryTools = false
	}
}
