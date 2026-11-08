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
	oauthdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/integrationoauth"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/mcpdb"
)

const emptyMCPEnvHeadersJSON = "[]"

func (s *Store) ListMCPServers() ([]mcpconfig.Server, error) {
	rows, err := mcpdb.New(s.db).ListMCPServers(context.Background())
	if err != nil {
		return nil, err
	}
	servers := make([]mcpconfig.Server, 0, len(rows))
	for _, row := range rows {
		server, err := s.decodeMCPServer(mcpdb.GetMCPServerRow(row))
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (s *Store) LoadMCPServer(id string) (mcpconfig.Server, error) {
	row, err := mcpdb.New(s.db).GetMCPServer(context.Background(), id)
	if err != nil {
		return mcpconfig.Server{}, mcpServerError(err)
	}
	return s.decodeMCPServer(row)
}

func (s *Store) CreateMCPServer(input mcpconfig.ServerInput) (mcpconfig.Server, error) {
	input, err := mcpconfig.ValidateInput(input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	id := newMCPServerID()
	config, err := s.encodeMCPConfig(id, input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	now := time.Now().UTC()
	server := input.Server(id, now, now)
	s.writeMu.Lock()
	err = mcpdb.New(s.db).CreateMCPServer(context.Background(), mcpdb.CreateMCPServerParams{
		ID:                server.ID,
		Name:              server.Name,
		Transport:         server.Transport,
		Url:               server.URL,
		Enabled:           boolInt(server.Enabled),
		BearerTokenEnvVar: nullDBString(server.BearerTokenEnvVar),
		BearerToken:       nullDBString(config.bearer),
		HeadersJson:       config.headers,
		EnvHeadersJson:    emptyMCPEnvHeadersJSON,
		OauthJson:         config.oauth,
		StdioJson:         config.stdio,
		CreatedAtMs:       timeToMs(server.CreatedAt),
		UpdatedAtMs:       timeToMs(server.UpdatedAt),
	})
	s.writeMu.Unlock()
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
	config, err := s.encodeMCPConfig(id, input)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	now := time.Now().UTC()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	defer tx.Rollback()
	mcpq := mcpdb.New(s.db).WithTx(tx)
	oauthq := oauthdb.New(s.db).WithTx(tx)
	current, err := mcpq.GetMCPServer(context.Background(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
		}
		return mcpconfig.Server{}, err
	}
	changed, err := mcpq.UpdateMCPServer(context.Background(), mcpdb.UpdateMCPServerParams{
		Name:              input.Name,
		Transport:         input.Transport,
		Url:               input.URL,
		Enabled:           boolInt(input.Enabled),
		BearerTokenEnvVar: nullDBString(input.BearerTokenEnvVar),
		BearerToken:       nullDBString(config.bearer),
		HeadersJson:       config.headers,
		EnvHeadersJson:    emptyMCPEnvHeadersJSON,
		OauthJson:         config.oauth,
		StdioJson:         config.stdio,
		UpdatedAtMs:       timeToMs(now),
		ID:                id,
	})
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	if current.Transport != input.Transport || current.Url != input.URL || current.OauthJson != config.oauth {
		if err := oauthq.DeleteToken(context.Background(), mcpconfig.OAuthConnectionID(id)); err != nil {
			return mcpconfig.Server{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return mcpconfig.Server{}, err
	}
	return input.Server(id, msToTime(current.CreatedAtMs), now), nil
}

func (s *Store) DeleteMCPServer(id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	mcpq := mcpdb.New(s.db).WithTx(tx)
	oauthq := oauthdb.New(s.db).WithTx(tx)
	changed, err := mcpq.DeleteMCPServer(context.Background(), id)
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("mcp server not found: %s", id)
	}
	if err := oauthq.DeleteToken(context.Background(), mcpconfig.OAuthConnectionID(id)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetMCPServerEnabled(id string, enabled bool) (mcpconfig.Server, error) {
	s.writeMu.Lock()
	changed, err := mcpdb.New(s.db).SetMCPServerEnabled(context.Background(), mcpdb.SetMCPServerEnabledParams{
		Enabled:     boolInt(enabled),
		UpdatedAtMs: timeToMs(time.Now().UTC()),
		ID:          id,
	})
	s.writeMu.Unlock()
	if err != nil {
		return mcpconfig.Server{}, err
	}
	if changed == 0 {
		return mcpconfig.Server{}, fmt.Errorf("mcp server not found: %s", id)
	}
	return s.LoadMCPServer(id)
}

// decodeMCPServer maps a stored row; List and Get select the same columns, so
// their row types convert to each other.
func (s *Store) decodeMCPServer(row mcpdb.GetMCPServerRow) (mcpconfig.Server, error) {
	bearer, err := s.decryptSecret("mcp:"+row.ID+":bearer", row.BearerToken.String)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	headers, err := s.decryptSecret("mcp:"+row.ID+":headers", row.HeadersJson)
	if err != nil {
		return mcpconfig.Server{}, err
	}
	server := mcpconfig.Server{
		ID:                row.ID,
		Name:              row.Name,
		Transport:         row.Transport,
		URL:               row.Url,
		Enabled:           row.Enabled != 0,
		BearerTokenEnvVar: row.BearerTokenEnvVar.String,
		BearerToken:       string(bearer),
		CreatedAt:         msToTime(row.CreatedAtMs),
		UpdatedAt:         msToTime(row.UpdatedAtMs),
	}
	if err := json.Unmarshal(headers, &server.Headers); err != nil {
		return mcpconfig.Server{}, err
	}
	if err := json.Unmarshal([]byte(row.OauthJson), &server.OAuth); err != nil {
		return mcpconfig.Server{}, err
	}
	if row.StdioJson != "" {
		data, err := s.decryptSecret("mcp:"+row.ID+":stdio", row.StdioJson)
		if err != nil {
			return mcpconfig.Server{}, err
		}
		var config stdioConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return mcpconfig.Server{}, err
		}
		server.Command = config.Command
		server.Args = config.Args
		server.Env = config.Env
	}
	return server, nil
}

type stdioConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func mcpServerError(err error) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("mcp server not found")
	}
	return err
}

type encodedMCPConfig struct {
	bearer  string
	headers string
	oauth   string
	stdio   string
}

func (s *Store) encodeMCPConfig(id string, input mcpconfig.ServerInput) (encodedMCPConfig, error) {
	var config encodedMCPConfig
	headers, err := json.Marshal(input.Headers)
	if err != nil {
		return config, err
	}
	oauth, err := json.Marshal(input.OAuth)
	if err != nil {
		return config, err
	}
	config.bearer, err = s.encryptSecret("mcp:"+id+":bearer", []byte(input.BearerToken))
	if err != nil {
		return config, err
	}
	config.headers, err = s.encryptSecret("mcp:"+id+":headers", headers)
	if err != nil {
		return config, err
	}
	config.oauth = string(oauth)
	if input.Transport == mcpconfig.TransportStdio {
		data, err := json.Marshal(stdioConfig{Command: input.Command, Args: input.Args, Env: input.Env})
		if err != nil {
			return config, err
		}
		config.stdio, err = s.encryptSecret("mcp:"+id+":stdio", data)
		if err != nil {
			return config, err
		}
	}
	return config, nil
}

func newMCPServerID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("mcp_%d", time.Now().UTC().UnixNano())
	}
	return "mcp_" + hex.EncodeToString(b[:])
}
