package sqlite

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	mcpconfig "github.com/wins/jaz/backend/internal/mcpconfig"
)

func (s *Store) ListMCPServers() ([]mcpconfig.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, name, transport, url, enabled, bearer_token_env_var,
headers_json, env_headers_json, created_at_ms, updated_at_ms
FROM mcp_servers ORDER BY updated_at_ms DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []mcpconfig.Server
	for rows.Next() {
		server, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) LoadMCPServer(id string) (mcpconfig.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id, name, transport, url, enabled, bearer_token_env_var,
headers_json, env_headers_json, created_at_ms, updated_at_ms
FROM mcp_servers WHERE id = ?`, id)
	return scanMCPServer(row)
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
	_, err = s.db.Exec(`INSERT INTO mcp_servers (
id, name, transport, url, enabled, bearer_token_env_var, headers_json, env_headers_json,
created_at_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		server.ID, server.Name, server.Transport, server.URL, boolInt(server.Enabled),
		nullString(server.BearerTokenEnvVar), headers, envHeaders, timeToMs(server.CreatedAt), timeToMs(server.UpdatedAt))
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
	res, err := s.db.Exec(`UPDATE mcp_servers SET
name = ?, transport = ?, url = ?, enabled = ?, bearer_token_env_var = ?,
headers_json = ?, env_headers_json = ?, updated_at_ms = ?
WHERE id = ?`,
		input.Name, mcpconfig.TransportStreamableHTTP, input.URL, boolInt(input.Enabled),
		nullString(input.BearerTokenEnvVar), headers, envHeaders, timeToMs(now), id)
	s.mu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return s.LoadMCPServer(id)
}

func (s *Store) DeleteMCPServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM mcp_servers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		return fmt.Errorf("mcp server not found: %s", id)
	}
	_, _ = s.db.Exec(`DELETE FROM mcp_oauth_tokens WHERE server_id = ?`, id)
	return nil
}

func (s *Store) SetMCPServerEnabled(id string, enabled bool) (mcpconfig.Server, error) {
	s.mu.Lock()
	res, err := s.db.Exec(`UPDATE mcp_servers SET enabled = ?, updated_at_ms = ? WHERE id = ?`,
		boolInt(enabled), timeToMs(time.Now().UTC()), id)
	s.mu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return s.LoadMCPServer(id)
}

type mcpServerScanner interface {
	Scan(dest ...any) error
}

func scanMCPServer(row mcpServerScanner) (mcpconfig.Server, error) {
	var server mcpconfig.Server
	var enabled int
	var bearer sql.NullString
	var headersJSON, envHeadersJSON string
	var createdMS, updatedMS int64
	if err := row.Scan(&server.ID, &server.Name, &server.Transport, &server.URL, &enabled, &bearer,
		&headersJSON, &envHeadersJSON, &createdMS, &updatedMS); err != nil {
		if err == sql.ErrNoRows {
			return mcpconfig.Server{}, fmt.Errorf("mcp server not found")
		}
		return mcpconfig.Server{}, err
	}
	server.Enabled = enabled != 0
	server.BearerTokenEnvVar = bearer.String
	server.CreatedAt = msToTime(createdMS)
	server.UpdatedAt = msToTime(updatedMS)
	if err := json.Unmarshal([]byte(headersJSON), &server.Headers); err != nil {
		return mcpconfig.Server{}, err
	}
	if err := json.Unmarshal([]byte(envHeadersJSON), &server.EnvHeaders); err != nil {
		return mcpconfig.Server{}, err
	}
	return server, nil
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

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
