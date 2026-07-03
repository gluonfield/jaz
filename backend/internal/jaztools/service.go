package jaztools

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsertask"
	"github.com/wins/jaz/backend/internal/browserworker"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/loops"
	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessiongoal"
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
	surfaceQueryParam        = "jaztools_surface"
	widgetSurfaceName        = "widget"
	memorySearchSurfaceName  = "memory_search_worker"
	memorySourceSurfaceName  = "memory_source_worker"
	browserWorkerSurfaceName = "browser_worker"
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
	Memory         *memoryservice.Service
	Browser        *browsertask.Service
	browserBackend browserworker.Backend

	loopTools       *loops.MCPTools
	agentTools      *acp.MCPTools
	threadTools     *threads.Service
	goalTools       *sessiongoal.MCPTools
	calendarTools   *connections.CalendarMCPTools
	gmailTools      *connections.GmailMCPTools
	whatsAppTools   *connections.WhatsAppMCPTools
	telegramTools   *connections.TelegramMCPTools
	visualizeTools  *visualize.MCPTools
	widgetPublisher *widgets.SessionPublisher
	sessions        sessionSource

	url string

	mu          sync.Mutex
	thread      serverSlot
	widget      serverSlot
	search      serverSlot
	source      serverSlot
	browser     serverSlot
	handlerOnce sync.Once
	handler     http.Handler
}

type toolSurface int

const (
	threadSurface toolSurface = iota
	widgetSurface
	searchWorkerSurface
	sourceWorkerSurface
	browserWorkerSurface
)

type surfaceSlot struct {
	surface toolSurface
	slot    *serverSlot
}

type serverSlot struct {
	once         sync.Once
	server       *mcp.Server
	memoryTools  bool
	agentTools   bool
	browserTools bool
	threadTools  bool
}

type sessionSource interface {
	LoadSession(id string) (storage.Session, error)
}

type goalStoreAdapter struct {
	storage.SessionStore
	storage.SessionEventAppender
	storage.UsageEventStore
}

func New(
	memory *memoryservice.Service,
	urls serverconfig.URLs,
	sessionEvents storage.SessionEventAppender,
	events *sessionevents.Bus,
	sessions storage.SessionStore,
	usage storage.UsageEventStore,
	widgetPublisher *widgets.SessionPublisher,
	calendarTools *connections.CalendarMCPTools,
	gmailTools *connections.GmailMCPTools,
	whatsAppTools *connections.WhatsAppMCPTools,
	telegramTools *connections.TelegramMCPTools,
) *Service {
	return &Service{
		Memory:          memory,
		calendarTools:   calendarTools,
		gmailTools:      gmailTools,
		whatsAppTools:   whatsAppTools,
		telegramTools:   telegramTools,
		visualizeTools:  visualize.NewMCPTools(sessionEvents, events),
		goalTools:       newGoalTools(sessionEvents, events, sessions, usage),
		widgetPublisher: widgetPublisher,
		sessions:        sessions,
		url:             strings.TrimSpace(urls.JazToolsMCP),
	}
}

func newGoalTools(sessionEvents storage.SessionEventAppender, events *sessionevents.Bus, sessions storage.SessionStore, usage storage.UsageEventStore) *sessiongoal.MCPTools {
	store := goalStoreAdapter{
		SessionStore:         sessions,
		SessionEventAppender: sessionEvents,
		UsageEventStore:      usage,
	}
	return sessiongoal.NewMCPTools(sessiongoal.New(store, events))
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

func (s *Service) SetBrowser(service *browsertask.Service, backend browserworker.Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Browser = service
	s.browserBackend = backend
	if s.browser.server != nil && s.browser.browserTools {
		browserworker.RemoveMCPTools(s.browser.server)
		browserworker.RemoveHighLevelMCPTools(s.browser.server)
		s.browser.browserTools = false
	}
	s.syncBrowserTools()
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
		s.syncBrowserToolsFor(slot, surface)
		s.mu.Unlock()
	})
	return slot.server
}

func (s *Service) slots() []surfaceSlot {
	return []surfaceSlot{
		{surface: threadSurface, slot: &s.thread},
		{surface: widgetSurface, slot: &s.widget},
		{surface: searchWorkerSurface, slot: &s.search},
		{surface: sourceWorkerSurface, slot: &s.source},
		{surface: browserWorkerSurface, slot: &s.browser},
	}
}

func (s *Service) slot(surface toolSurface) *serverSlot {
	switch surface {
	case browserWorkerSurface:
		return &s.browser
	case searchWorkerSurface:
		return &s.search
	case sourceWorkerSurface:
		return &s.source
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
	if surface.workerOnly() {
		return server
	}
	s.loopTools.AddTo(server)
	if surface == threadSurface && s.goalTools != nil {
		s.goalTools.AddTo(server)
	}
	s.calendarTools.AddTo(server)
	s.gmailTools.AddTo(server)
	s.whatsAppTools.AddTo(server)
	s.telegramTools.AddTo(server)
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
	s.syncBrowserTools()
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
	case browserWorkerSurfaceName:
		return browserWorkerSurface
	case memorySearchSurfaceName:
		return searchWorkerSurface
	case memorySourceSurfaceName:
		return sourceWorkerSurface
	case widgetSurfaceName:
		return widgetSurface
	}
	if session, ok := s.sessionFromRequest(r); ok {
		if workerSurface, found := workerSurfaceBySourceType[session.SourceType]; found {
			return workerSurface
		}
	}
	if s.widgetSession(r) {
		return widgetSurface
	}
	return threadSurface
}

// workerSurfaceBySourceType routes a backend worker session, identified by its
// source type, to its restricted tool surface. Adding a worker is one entry.
var workerSurfaceBySourceType = map[string]toolSurface{
	storage.SourceMemorySearch: searchWorkerSurface,
	storage.SourceMemorySource: sourceWorkerSurface,
	storage.SourceBrowserTask:  browserWorkerSurface,
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
	for _, current := range s.slots() {
		s.syncMemoryToolsFor(current.slot, current.surface)
	}
}

func (s *Service) syncAgentTools() {
	for _, current := range s.slots() {
		s.syncAgentToolsFor(current.slot, current.surface)
	}
}

func (s *Service) syncBrowserTools() {
	for _, current := range s.slots() {
		s.syncBrowserToolsFor(current.slot, current.surface)
	}
}

func (s *Service) syncThreadTools() {
	s.syncThreadToolsFor(&s.thread, threadSurface)
	s.syncThreadToolsFor(&s.widget, widgetSurface)
}

func (s *Service) syncAgentToolsFor(slot *serverSlot, surface toolSurface) {
	if !surface.agentToolsAllowed() || slot.server == nil || slot.agentTools || s.agentTools == nil {
		return
	}
	s.agentTools.AddTo(slot.server)
	slot.agentTools = true
}

func (s *Service) syncThreadToolsFor(slot *serverSlot, surface toolSurface) {
	if !surface.threadToolsAllowed() || slot.server == nil || slot.threadTools || s.threadTools == nil {
		return
	}
	s.threadTools.AddMCPTools(slot.server)
	slot.threadTools = true
}

func (s *Service) syncMemoryToolsFor(slot *serverSlot, surface toolSurface) {
	if !surface.memoryToolsAllowed() || slot.server == nil {
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

func (s *Service) syncBrowserToolsFor(slot *serverSlot, surface toolSurface) {
	if !surface.browserToolsAllowed() || slot.server == nil || s.Browser == nil {
		return
	}
	if s.Browser.MCPToolsEnabled() {
		if !slot.browserTools {
			s.addBrowserTools(slot.server, surface)
			slot.browserTools = true
		}
		return
	}
	if slot.browserTools {
		s.removeBrowserTools(slot.server, surface)
		slot.browserTools = false
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

func (s *Service) addBrowserTools(server *mcp.Server, surface toolSurface) {
	switch {
	case surface.browserWorkerTools():
		browserworker.AddMCPTools(server, s.browserBackend)
		browserworker.AddHighLevelMCPTools(server, s.browserBackend)
	case surface.browserTaskToolsAllowed():
		s.Browser.AddMCPTools(server)
	}
}

func (s *Service) removeBrowserTools(server *mcp.Server, surface toolSurface) {
	switch {
	case surface.browserWorkerTools():
		browserworker.RemoveMCPTools(server)
		browserworker.RemoveHighLevelMCPTools(server)
	case surface.browserTaskToolsAllowed():
		s.Browser.RemoveMCPTools(server)
	}
}

func (surface toolSurface) workerOnly() bool {
	return surface == searchWorkerSurface || surface == sourceWorkerSurface || surface == browserWorkerSurface
}

func (surface toolSurface) agentToolsAllowed() bool {
	return surface == threadSurface || surface == widgetSurface
}

func (surface toolSurface) threadToolsAllowed() bool {
	return surface == threadSurface || surface == widgetSurface
}

func (surface toolSurface) memoryToolsAllowed() bool {
	return surface != browserWorkerSurface
}

func (surface toolSurface) browserWorkerTools() bool {
	return surface == browserWorkerSurface
}

func (surface toolSurface) browserToolsAllowed() bool {
	return surface.browserWorkerTools() || surface.browserTaskToolsAllowed()
}

func (surface toolSurface) browserTaskToolsAllowed() bool {
	return surface == threadSurface || surface == widgetSurface
}
