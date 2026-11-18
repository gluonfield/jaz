package mcp

import (
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func (m *Manager) Run() {
	m.Refresh(m.ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			_ = m.refresh(m.ctx, func(server mcpconfig.Server) bool {
				status := m.Status(server.ID)
				return status.Status == "" || status.Retryable
			})
		}
	}
}
