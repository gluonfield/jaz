package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/subprocess"
)

func (m *Manager) connectStdio(ctx context.Context, server mcpconfig.Server) (*serverSession, error) {
	// The connection owns the process, not the short-lived discovery context.
	command := exec.Command(server.Command, server.Args...)
	command.Env = os.Environ()
	for name, value := range server.Env {
		command.Env = append(command.Env, name+"="+value)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "jaz", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, stdioTransport{command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect local MCP server: %w", err)
	}
	// A disconnected process may have already executed a tool: never replay it.
	connection := &serverConnection{session: session}
	ss, err := connection.loadCatalog(ctx, server)
	if err != nil {
		connection.close()
	}
	return ss, err
}

type stdioTransport struct {
	command *exec.Cmd
}

func (t stdioTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	process := subprocess.New(t.command)
	connection, err := (&mcpsdk.CommandTransport{Command: t.command, TerminateDuration: 2 * time.Second}).Connect(ctx)
	if err != nil {
		return nil, err
	}
	managed := &stdioConnection{Connection: connection, process: process}
	if err := process.Started(); err != nil {
		_ = managed.Close()
		return nil, err
	}
	return managed, nil
}

type stdioConnection struct {
	mcpsdk.Connection
	process *subprocess.Supervisor
	once    sync.Once
	err     error
}

func (c *stdioConnection) Close() error {
	c.once.Do(func() {
		c.err = c.Connection.Close()
		if err := c.process.Terminate(); c.err == nil {
			c.err = err
		}
	})
	return c.err
}
