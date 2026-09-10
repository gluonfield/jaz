package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestReadTextFileLineContract(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "lines.txt")
	content := "alpha\nβeta\r\n\ngamma"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, Config{}, nil)
	job := &jobState{Job: Job{ID: "test", ACPSession: "test-acp", Cwd: dir}}
	manager.addJob(job, nil)
	for _, tt := range []struct {
		name  string
		line  int
		limit int
		want  string
	}{
		{"whole file", 0, 0, content},
		{"first line", 0, 1, "alpha\n"},
		{"unicode and CRLF", 2, 1, "βeta\r\n"},
		{"blank line", 3, 1, "\n"},
		{"remaining lines", 2, 0, "βeta\r\n\ngamma"},
		{"past end", 9, 1, ""},
		{"limit past end", 4, 99, "gamma"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, rpcErr := manager.handleJSONRPC(context.Background(), jsonrpc.Request{
				Method: "fs/read_text_file",
				Params: mustJSON(t, map[string]any{
					"sessionId": job.ACPSession,
					"path":      file,
					"line":      tt.line,
					"limit":     tt.limit,
				}),
			})
			if rpcErr != nil {
				t.Fatal(rpcErr)
			}
			var response struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.Content != tt.want {
				t.Fatalf("content = %q, want %q", response.Content, tt.want)
			}
		})
	}
}
