package mcp

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

type recoveryInventory struct {
	mu      sync.Mutex
	servers []mcpconfig.Server
}

func (s *recoveryInventory) ListMCPServers() ([]mcpconfig.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.servers), nil
}

func (s *recoveryInventory) replace(servers []mcpconfig.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.servers = slices.Clone(servers)
}

func TestManagerRecoversUnavailableCatalog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var unavailable atomic.Bool
		unavailable.Store(true)
		var healthyReads atomic.Int32
		makeServer := func(id string, fail *atomic.Bool, reads *atomic.Int32) mcpconfig.Server {
			remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: id}, nil)
			remote.AddTool(&mcpsdk.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: id}}}, nil
			})
			remote.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
				return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
					if method == "tools/list" {
						reads.Add(1)
						if fail.Load() {
							return nil, errors.New("temporarily unavailable")
						}
					}
					return next(ctx, method, req)
				}
			})
			return mcpconfig.Server{ID: id, Name: id, Enabled: true, LocalServer: remote}
		}
		var healthyFailure atomic.Bool
		var unavailableReads atomic.Int32
		servers := []mcpconfig.Server{
			makeServer("healthy", &healthyFailure, &healthyReads),
			makeServer("recovering", &unavailable, &unavailableReads),
		}
		manager := NewManager(&testStore{servers: servers}, nil, tools.NewRegistry(), log.New(io.Discard))
		defer manager.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go manager.Run()
		synctest.Wait()
		if manager.Status("recovering").Status != "error" || manager.Status("healthy").Status != "connected" {
			t.Fatalf("initial statuses: recovering=%+v healthy=%+v", manager.Status("recovering"), manager.Status("healthy"))
		}
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		proxy, err := manager.proxy.Connect(ctx, serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer proxy.Close()
		var changes atomic.Int32
		proxyClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test"}, &mcpsdk.ClientOptions{
			ToolListChangedHandler: func(context.Context, *mcpsdk.ToolListChangedRequest) {
				changes.Add(1)
			},
		})
		client, err := proxyClient.Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		initial, err := client.ListTools(ctx, nil)
		if err != nil || len(initial.Tools) != 1 {
			t.Fatalf("initial proxy catalog: %+v %v", initial, err)
		}
		healthy := manager.sessions["healthy"].serverConnection
		revision := manager.Revision()
		time.Sleep(time.Minute)
		synctest.Wait()
		if manager.Revision() != revision || changes.Load() != 0 {
			t.Fatal("unchanged failure invalidated the catalog")
		}
		if unavailableReads.Load() < 2 {
			t.Fatal("unavailable provider was not retried")
		}
		unavailable.Store(false)
		time.Sleep(30 * time.Second)
		synctest.Wait()
		recovered, err := client.ListTools(ctx, nil)
		if err != nil || len(recovered.Tools) != 2 {
			t.Fatalf("proxy did not recover the unavailable provider: %+v %v", recovered, err)
		}
		if healthyReads.Load() != 1 || manager.sessions["healthy"].serverConnection != healthy {
			t.Fatal("recovery refreshed the healthy provider")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if changes.Load() != 1 {
			t.Fatalf("recovery emitted %d catalog changes, want 1", changes.Load())
		}
		for _, tool := range recovered.Tools {
			result, err := client.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool.Name})
			if err != nil || result.IsError {
				t.Fatalf("recovered tool %q failed: %+v %v", tool.Name, result, err)
			}
		}
		reads := unavailableReads.Load()
		manager.Close()
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		if unavailableReads.Load() != reads {
			t.Fatal("recovery continued after shutdown")
		}
	})
}

func TestRecoveryStopsForDisabledAndRemovedProviders(t *testing.T) {
	for _, mode := range []string{"disabled", "removed"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var reads atomic.Int32
				remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "unavailable"}, nil)
				remote.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
					return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
						if method == "tools/list" {
							reads.Add(1)
							return nil, errors.New("temporarily unavailable")
						}
						return next(ctx, method, req)
					}
				})
				server := mcpconfig.Server{ID: "unavailable", Name: "unavailable", Enabled: true, LocalServer: remote}
				store := &recoveryInventory{servers: []mcpconfig.Server{server}}
				manager := NewManager(store, nil, tools.NewRegistry(), log.New(io.Discard))
				defer manager.Close()
				go manager.Run()
				synctest.Wait()
				if manager.Status("unavailable").Status != "error" || reads.Load() != 1 {
					t.Fatal("initial unavailable provider was not checked")
				}
				if mode == "disabled" {
					server.Enabled = false
					store.replace([]mcpconfig.Server{server})
				} else {
					store.replace(nil)
				}
				time.Sleep(time.Minute)
				synctest.Wait()
				if reads.Load() != 1 {
					t.Fatalf("%s provider was retried", mode)
				}
				status := manager.Status("unavailable").Status
				if (mode == "disabled" && status != "disabled") || (mode == "removed" && status != "") {
					t.Fatalf("%s provider status = %q", mode, status)
				}
			})
		})
	}
}
