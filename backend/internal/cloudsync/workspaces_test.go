package cloudsync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudWorkspacesCollapseDuplicatesOnLoad(t *testing.T) {
	ctx := t.Context()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	accounts := []storage.CloudAccount{
		{ID: "stale", BaseURL: "https://cloud.example", OrgID: "org-a", UserID: "old", ConnectedAt: time.Unix(1, 0)},
		{ID: "live", BaseURL: " https://CLOUD.example:443/ ", OrgID: "org-a", UserID: "new", Token: "current", ConnectedAt: time.Unix(2, 0)},
		{ID: "other-org", BaseURL: "https://cloud.example", OrgID: "org-b", Token: "other"},
		{ID: "other-server", BaseURL: "https://other.example", OrgID: "org-a", Token: "other"},
	}
	for _, account := range accounts {
		if err := store.SaveCloudAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveActiveWorkspaceID(ctx, "stale"); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		status := cloudsync.New(store, nil).Status(ctx)
		if !status.Connected || status.ActiveWorkspaceID != "live" || status.URL != "https://cloud.example" || len(status.Workspaces) != 3 {
			t.Fatalf("deduplicated status: %+v", status)
		}
	}
	if id, err := store.LoadActiveWorkspaceID(ctx); err != nil || id != "live" {
		t.Fatalf("saved selection = %q, %v", id, err)
	}
	if _, found, err := store.LoadCloudAccount(ctx, "stale"); err != nil || found {
		t.Fatalf("obsolete account still persisted: found=%v err=%v", found, err)
	}
}

func TestCloudWorkspacesReuseServerAndOrgOnSignIn(t *testing.T) {
	ctx := t.Context()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/exchange":
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid exchange", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": body.Code, "refresh_token": "refresh-" + body.Code})
		case "/v1/auth/me":
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			user, _, _ := strings.Cut(token, "-")
			_ = json.NewEncoder(w).Encode(map[string]string{"user_id": user, "org_id": "org-a"})
		case "/v1/auth/logout":
			t.Error("replacing a workspace must not sign out of Cloud")
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer upstream.Close()
	base := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
	variant := " " + strings.Replace(base, "localhost", "LOCALHOST", 1) + "/ "

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := cloudsync.New(store, nil)
	first, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: variant, Code: "alice-first"})
	if err != nil || !first.Connected || len(first.Workspaces) != 1 || first.URL != base {
		t.Fatalf("first sign-in = %+v, %v", first, err)
	}
	if _, err := svc.SwitchWorkspace(ctx, cloudsync.LocalWorkspaceID); err != nil {
		t.Fatal(err)
	}
	relog, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: base, Code: "alice-second"})
	if err != nil || !relog.Connected || len(relog.Workspaces) != 1 || relog.ActiveWorkspaceID != first.ActiveWorkspaceID {
		t.Fatalf("sign in again = %+v, %v", relog, err)
	}
	if account, found, err := store.LoadCloudAccount(ctx, relog.ActiveWorkspaceID); err != nil || !found || account.RefreshToken != "refresh-alice-second" {
		t.Fatalf("refreshed credentials persisted: found=%v err=%v", found, err)
	}

	replaced, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: variant, Code: "bob-first"})
	if err != nil || !replaced.Connected || len(replaced.Workspaces) != 1 || replaced.ActiveWorkspaceID == relog.ActiveWorkspaceID || replaced.User.ID != "bob" {
		t.Fatalf("signed in as another principal = %+v, %v", replaced, err)
	}
	accounts, err := store.ListCloudAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].ID != replaced.ActiveWorkspaceID {
		t.Fatalf("saved accounts after replacement: count=%d err=%v", len(accounts), err)
	}
}
