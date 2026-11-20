package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDisconnectMuseAllowsNativeKeylessEndpoint(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"META_API_KEY", "JAZ_ACP_MUSE_API_KEY"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := disconnectTestServer(store, root, acp.AgentMuse)
	s.AgentCatalog[acp.AgentMuse] = acp.AgentConfig{
		Command: executable,
		Env: map[string]string{
			"MUSE_CLI":           executable,
			"MUSE_JAZ_TEST_HTTP": "1",
			"XDG_CONFIG_HOME":    root,
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/muse/auth/disconnect", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("disconnect status = %d, body = %s", res.Code, res.Body.String())
	}
	var got acpDisconnectResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	auth := got.Auth
	if !auth.Authenticated || auth.AuthKind != acp.AuthKindNone {
		t.Fatalf("native keyless endpoint became unavailable after logout: %#v", auth)
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("MUSE_JAZ_TEST_HTTP") == "" {
		os.Exit(m.Run())
	}
	if len(os.Args) == 2 && os.Args[1] == "logout" {
		os.Exit(0)
	}
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		os.Exit(2)
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		if len(request.ID) == 0 {
			continue
		}
		result := json.RawMessage(`{}`)
		if request.Method == "account/read" {
			result = json.RawMessage(`{"state":"loggedOut","credentialRequired":false}`)
		}
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			os.Exit(3)
		}
	}
}
