package connections

import (
	"context"
	"time"
)

const mcpRefreshTimeout = 30 * time.Second

type MCPRefresher interface {
	Refresh(context.Context)
}

func refreshMCP(refresher MCPRefresher) {
	if refresher == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mcpRefreshTimeout)
		defer cancel()
		refresher.Refresh(ctx)
	}()
}
