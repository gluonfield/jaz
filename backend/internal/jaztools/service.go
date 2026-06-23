package jaztools

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/visualize"
	"github.com/wins/jaz/backend/internal/widgets"
)

const (
	ServerID   = "jaztools"
	ServerName = "jaztools"
	Version    = "0.1.0"
)

const (
	surfaceQueryParam       = "jaztools_surface"
	widgetSurfaceName       = "widget"
	memorySearchSurfaceName = "memory_search_worker"
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
	agentTools      *acp.MCPTools
	threadTools     *threads.Service
	visualizeTools  *visualize.MCPTools
	widgetPublisher *widgets.SessionPublisher
	sessions        sessionSource

	url string

	mu          sync.Mutex
	thread      serverSlot
	widget      serverSlot
	search      serverSlot
	handlerOnce sync.Once
	handler     http.Handler
}

type toolSurface int

const (
	threadSurface toolSurface = iota
	widgetSurface
	searchWorkerSurface
)

type serverSlot struct {
	once        sync.Once
	server      *mcp.Server
	memoryTools bool
	agentTools  bool
	threadTools bool
}

type sessionSource interface {
	LoadSession(id string) (storage.Session, error)
}

func New(memory *memoryservice.Service, urls serverconfig.URLs, sessionEvents storage.SessionEventAppender, events *sessionevents.Bus, sessions storage.SessionStore, widgetPublisher *widgets.SessionPublisher) *Service {
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

func (s *Service) SetLoops(service loops.MCPService, opts ...loops.MCPOption) {
	s.loopTools = loops.NewMCPTools(service, opts...)
}

func (s *Service) SetAgents(service acp.MCPService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentTools = acp.NewMCPTools(service)
	s.syncAgentTools()
}

func (s *Service) SetThreads(service *threads.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threadTools = service
	s.syncThreadTools()
}

func (s *Service) Server() *mcp.Server {
	return s.server(threadSurface)
}

func (s *Service) server(surface toolSurface) *mcp.Server {
	slot := s.slot(surface)
	slot.once.Do(func() {
		server := s.newServer(surface)
		s.mu.Lock()
		slot.server = server
		s.syncAgentToolsFor(slot, surface)
		s.syncThreadToolsFor(slot, surface)
		s.syncMemoryToolsFor(slot, surface)
		s.mu.Unlock()
	})
	return slot.server
}

func (s *Service) slot(surface toolSurface) *serverSlot {
	switch surface {
	case searchWorkerSurface:
		return &s.search
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
	if surface == searchWorkerSurface {
		return server
	}
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
	s.syncThreadTools()
	s.syncAgentTools()
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
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(surfaceQueryParam))) {
	case memorySearchSurfaceName:
		return searchWorkerSurface
	case widgetSurfaceName:
		return widgetSurface
	}
	if s.searchWorkerSession(r) {
		return searchWorkerSurface
	}
	if s.widgetSession(r) {
		return widgetSurface
	}
	return threadSurface
}

func (s *Service) searchWorkerSession(r *http.Request) bool {
	session, ok := s.sessionFromRequest(r)
	return ok && session.SourceType == storage.SourceMemorySearch
}

func (s *Service) widgetSession(r *http.Request) bool {
	session, ok := s.sessionFromRequest(r)
	return ok &&
		s.widgetPublisher != nil &&
		s.widgetPublisher.WidgetSurfaceForSession(session.ID)
}

func (s *Service) sessionFromRequest(r *http.Request) (storage.Session, bool) {
	sessionID := strings.TrimSpace(r.Header.Get(mcpsession.HeaderName))
	if sessionID == "" || s.sessions == nil {
		return storage.Session{}, false
	}
	session, err := s.sessions.LoadSession(sessionID)
	if err != nil {
		return storage.Session{}, false
	}
	return session, true
}

func (s *Service) syncMemoryTools() {
	s.syncMemoryToolsFor(&s.thread, threadSurface)
	s.syncMemoryToolsFor(&s.widget, widgetSurface)
	s.syncMemoryToolsFor(&s.search, searchWorkerSurface)
}

func (s *Service) syncAgentTools() {
	s.syncAgentToolsFor(&s.thread, threadSurface)
	s.syncAgentToolsFor(&s.widget, widgetSurface)
}

func (s *Service) syncThreadTools() {
	s.syncThreadToolsFor(&s.thread, threadSurface)
	s.syncThreadToolsFor(&s.widget, widgetSurface)
}

func (s *Service) syncAgentToolsFor(slot *serverSlot, surface toolSurface) {
	if surface == searchWorkerSurface || slot.server == nil || slot.agentTools || s.agentTools == nil {
		return
	}
	s.agentTools.AddTo(slot.server)
	slot.agentTools = true
}

func (s *Service) syncThreadToolsFor(slot *serverSlot, surface toolSurface) {
	if surface == searchWorkerSurface || slot.server == nil || slot.threadTools || s.threadTools == nil {
		return
	}
	s.threadTools.AddMCPTools(slot.server)
	slot.threadTools = true
}

func (s *Service) syncMemoryToolsFor(slot *serverSlot, surface toolSurface) {
	if slot.server == nil {
		return
	}
	if s.Memory.MCPToolsEnabled() {
		if !slot.memoryTools {
			s.addMemoryTools(slot.server, surface)
			slot.memoryTools = true
		}
		return
	}
	if slot.memoryTools {
		s.removeMemoryTools(slot.server, surface)
		slot.memoryTools = false
	}
}

func (s *Service) addMemoryTools(server *mcp.Server, surface toolSurface) {
	if surface == searchWorkerSurface {
		s.Memory.AddWorkerMCPTools(server)
		return
	}
	s.Memory.AddMCPTools(server)
}

func (s *Service) removeMemoryTools(server *mcp.Server, surface toolSurface) {
	if surface == searchWorkerSurface {
		s.Memory.RemoveWorkerMCPTools(server)
		return
	}
	s.Memory.RemoveMCPTools(server)
}
