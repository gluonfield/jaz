package jaztools

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsertask"
	"github.com/wins/jaz/backend/internal/connections"
	jazconnector "github.com/wins/jaz/backend/internal/connectors/jaz"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/memoryservice"
	"github.com/wins/jaz/backend/internal/serverconfig"
	"github.com/wins/jaz/backend/internal/sessionevents"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/threads"
	"github.com/wins/jaz/backend/internal/widgets"
)

type fakeBoards struct{}

func (fakeBoards) ListBoards() ([]loops.BoardSummary, error)   { return nil, nil }
func (fakeBoards) ValidateBoardIDs([]string) error             { return nil }
func (fakeBoards) AssignLoopBoards(loops.Loop, []string) error { return nil }
func (fakeBoards) BoardsForLoop(string) ([]string, error)      { return nil, nil }

// The jaz connection plugin hand-declares tool metadata that must mirror the
// thread-surface registrations here; this pins the two together.
func TestJazPluginToolsMatchThreadSurface(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := jazsettings.SaveBrowserSettings(store, jazsettings.BrowserSettings{Enabled: true, Agent: acp.AgentCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := jazsettings.SaveAgentDefaults(store, jazsettings.AgentDefaults{ACP: map[string]jazsettings.ACPAgentDefaults{
		acp.AgentCodex: {Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "memory.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	service := New(
		memoryservice.New(memory, store, fakeScheduler{}, "http://127.0.0.1:5299/mcp/jaztools"),
		serverconfig.URLs{JazToolsMCP: "http://127.0.0.1:5299/mcp/jaztools"},
		store,
		sessionevents.New(),
		store,
		store,
		&widgets.SessionPublisher{Service: widgets.NewService(store, nil), Sessions: store, Loops: store},
		testCalendarTools(t, store),
		testGmailTools(t, store),
		connections.NewWhatsAppMCPTools(store, nil, nil),
		connections.NewTelegramMCPTools(store, nil, nil),
	)
	service.SetLoops(loops.NewService(store, &fakeExecutor{started: make(chan loops.Run, 1)}, nil), loops.WithBoards(fakeBoards{}))
	service.SetThreads(threads.NewService(sqlitestore.NewSearchQueries(store), store))
	service.SetAgents(fakeACPService{spawned: make(chan acp.SpawnRequest, 1)})
	service.SetBrowser(browsertask.New(store, fakeACPService{spawned: make(chan acp.SpawnRequest, 1)}, acp.BuiltinAgents(), fakeBrowserBackend{}), fakeBrowserBackend{})

	session, closeSession := connectClient(t, service.Server())
	defer closeSession()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, tool := range tools.Tools {
		registered[tool.Name] = true
	}
	for _, tool := range jazconnector.Plugin().Tools {
		if !registered[tool.Name] {
			t.Fatalf("jaz plugin advertises %q but the thread surface does not register it", tool.Name)
		}
	}
}
