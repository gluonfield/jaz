package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type sseTransport struct {
	mcpsdk.SSEClientTransport
}

func (t sseTransport) Connect(ctx context.Context) (mcpsdk.Connection, error) {
	// The SSE stream outlives discovery; the connection owns its cancellation.
	stream, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	connection, err := t.SSEClientTransport.Connect(stream)
	stop()
	if err != nil {
		cancel()
		return nil, err
	}
	return &sseConnection{Connection: connection, cancel: cancel}, nil
}

type sseConnection struct {
	mcpsdk.Connection
	cancel context.CancelFunc
}

func (c *sseConnection) Close() error {
	c.cancel()
	return c.Connection.Close()
}
