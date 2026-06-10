package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/mcpdb"
)

func (s *Store) ListMCPServers() ([]mcpconfig.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := mcpdb.New(s.db).ListMCPServers(context.Background())
	if err != nil {
		return nil, err
	}
	servers := make([]mcpconfig.Server, 0, len(rows))
	for _, row := range rows {
		server, err := mcpServerFromDB(row)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (s *Store) LoadMCPServer(id string) (mcpconfig.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := mcpdb.New(s.db).GetMCPServer(context.Background(), id)
	if err != nil {
		return mcpconfig.Server{}, mcpServerError(err)
	}
	return mcpServerFromDB(row)
}

func (s *Store) CreateMCPServer(input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	input, err := mcpconfig.ValidateInput(input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	headers, envHeaders, err := encodeMCPHeaders(input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	now := time.Now().UTC()
	server := mcpconfig.Server{
		ID:                newMCPServerID(),
		Name:              input.Name,
		Transport:         mcpconfig.TransportStreamableHTTP,
		URL:               input.URL,
		Enabled:           input.Enabled,
		BearerTokenEnvVar: input.BearerTokenEnvVar,
		Headers:           input.Headers,
		EnvHeaders:        input.EnvHeaders,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.mu.Lock()
	err = mcpdb.New(s.db).CreateMCPServer(context.Background(), mcpdb.CreateMCPServerParams{
		ID:                server.ID,
		Name:              server.Name,
		Transport:         server.Transport,
		Url:               server.URL,
		Enabled:           boolInt(server.Enabled),
		BearerTokenEnvVar: nullDBString(server.BearerTokenEnvVar),
		HeadersJson:       headers,
		EnvHeadersJson:    envHeaders,
		CreatedAtMs:       timeToMs(server.CreatedAt),
		UpdatedAtMs:       timeToMs(server.UpdatedAt),
	})
	s.mu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	return server, nil
}

func (s *Store) UpdateMCPServer(id string, input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	input, err := mcpconfig.ValidateInput(input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	headers, envHeaders, err := encodeMCPHeaders(input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	changed, err := mcpdb.New(s.db).UpdateMCPServer(context.Background(), mcpdb.UpdateMCPServerParams{
		Name:              input.Name,
		Transport:         mcpconfig.TransportStreamableHTTP,
		Url:               input.URL,
		Enabled:           boolInt(input.Enabled),
		BearerTokenEnvVar: nullDBString(input.BearerTokenEnvVar),
		HeadersJson:       headers,
		EnvHeadersJson:    envHeaders,
		UpdatedAtMs:       timeToMs(now),
		ID:                id,
	})
	s.mu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return s.LoadMCPServer(id)
}

func (s *Store) DeleteMCPServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := mcpdb.New(s.db)
	changed, err := q.DeleteMCPServer(context.Background(), id)
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("mcp server not found: %s", id)
	}
	_ = q.DeleteMCPOAuthToken(context.Background(), id)
	return nil
}

func (s *Store) SetMCPServerEnabled(id string, enabled bool) (mcpconfig.Server, error) {
	s.mu.Lock()
	changed, err := mcpdb.New(s.db).SetMCPServerEnabled(context.Background(), mcpdb.SetMCPServerEnabledParams{
		Enabled:     boolInt(enabled),
		UpdatedAtMs: timeToMs(time.Now().UTC()),
		ID:          id,
	})
	s.mu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return s.LoadMCPServer(id)
}

func mcpServerFromDB(row mcpdb.McpServer) (mcpconfig.Server, error) {
	server := mcpconfig.Server{
		ID:                row.ID,
		Name:              row.Name,
		Transport:         row.Transport,
		URL:               row.Url,
		Enabled:           row.Enabled != 0,
		BearerTokenEnvVar: row.BearerTokenEnvVar.String,
		CreatedAt:         msToTime(row.CreatedAtMs),
		UpdatedAt:         msToTime(row.UpdatedAtMs),
	}
	if err := json.Unmarshal([]byte(row.HeadersJson), &server.Headers); err != nil {
		return mcpconfig.Server{}, err
	}
	if err := json.Unmarshal([]byte(row.EnvHeadersJson), &server.EnvHeaders); err != nil {
		return mcpconfig.Server{}, err
	}
	return server, nil
}

func mcpServerError(err error) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("mcp server not found")
	}
	return err
}

func encodeMCPHeaders(input mcpconfig.ServerInput) (string, string, error) {
	headers, err := json.Marshal(input.Headers)
	if err != nil {
		return "", "", err
	}
	envHeaders, err := json.Marshal(input.EnvHeaders)
	if err != nil {
		return "", "", err
	}
	return string(headers), string(envHeaders), nil
}

func newMCPServerID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("mcp_%d", time.Now().UTC().UnixNano())
	}
	return "mcp_" + hex.EncodeToString(b[:])
}
