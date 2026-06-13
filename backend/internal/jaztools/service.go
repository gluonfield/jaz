package jaztools

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
)

const Version = "0.1.0"

type Service struct {
	Memory *memoryservice.Service

	loopTools *loops.MCPTools

	url string

	mu          sync.Mutex
	memoryTools bool
	serverOnce  sync.Once
	server      *mcp.Server
	handlerOnce sync.Once
	handler     http.Handler
}

func New(memory *memoryservice.Service, urls serverconfig.URLs) *Service {
	return &Service{Memory: memory, url: strings.TrimSpace(urls.JazToolsMCP)}
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
			Name:    "jaztools",
			Title:   "Jaz Tools",
			Version: Version,
		}, nil)
		s.loopTools.AddTo(server)
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
