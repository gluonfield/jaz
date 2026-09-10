package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/pathsafe"
)

func (m *Manager) readTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.ReadTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/read_text_file", map[string]any{"error": err.Error()})
	}
	if req.Line < 0 || req.Limit < 0 {
		return nil, jsonrpc.InvalidParams("line and limit must be positive", nil)
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := pathsafe.Resolve(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	lines := strings.SplitAfter(string(data), "\n")
	start := min(max(req.Line-1, 0), len(lines))
	lines = lines[start:]
	if req.Limit > 0 {
		lines = lines[:min(req.Limit, len(lines))]
	}
	content := strings.Join(lines, "")
	return jsonrpc.EncodeResult(acpschema.ReadTextFileResponse{Content: content})
}

func (m *Manager) writeTextFile(raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.WriteTextFileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid fs/write_text_file", map[string]any{"error": err.Error()})
	}
	job := m.jobByACP(string(req.SessionID))
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	path, err := pathsafe.Resolve(job.Cwd, req.Path)
	if err != nil {
		return nil, jsonrpc.InvalidParams(err.Error(), nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	if err := os.WriteFile(path, []byte(req.Content), 0o644); err != nil {
		return nil, jsonrpc.InternalError(err.Error(), nil)
	}
	return jsonrpc.EncodeResult(acpschema.WriteTextFileResponse{})
}
