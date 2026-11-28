package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/server"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDesktopCloudMultiRepoRoutes(t *testing.T) {
	const repos = `[{"repo":"demo/app","base_branch":"main"},{"repo":"demo/docs","base_branch":"release","mount_dir":"guide"}]`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response string
		switch r.Method + " " + r.URL.Path {
		case "GET /.well-known/jaz-server":
			response = `{"ok":true,"mode":"cloud","auth":"oidc"}`
		case "GET /v1/auth/me":
			response = `{"user_id":"owner","org_id":"demo","role":"member"}`
		case "GET /v1/spaces":
			response = `{"spaces":[{"id":"project","name":"Demo","default_git_repos":` + repos + `}]}`
		case "POST /v1/spaces", "POST /v1/threads":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.URL.Path == "/v1/spaces" {
				response = `{"id":"new-project","name":"Demo","default_git_repos":` + string(body["default_git_repos"]) + `}`
				break
			}
			want := map[string]string{`"Selected"`: repos, `"Cleared"`: "[]"}[string(body["title"])]
			if got := string(body["git_repos"]); got != want {
				t.Errorf("git_repos = %s, want %s", got, want)
			}
			response = `{"id":"thread","owner_user_id":"owner","run_id":"thread","status":"idle","visibility":"space"}`
		case "POST /v1/threads/thread/repos":
			var input cloudsync.ThreadGitRepoSpec
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Repo != "demo/docs" || input.MountDir != "guide" || input.BaseBranch != "release" {
				t.Errorf("unexpected upstream update: %+v, %v", input, err)
			}
			response = `{"ok":true}`
		case "DELETE /v1/threads/thread/repos/guide":
			response = `{"deleted":true}`
		case "GET /v1/threads/thread/repo/diff":
			if r.URL.Query().Get("repo") != "demo/docs" || r.URL.Query().Get("path") != "README.md" {
				t.Errorf("unexpected upstream diff query: %v", r.URL.Query())
			}
			response = `{"repo":"demo/docs","diffs":[]}`
		default:
			response = "{}"
		}
		_, _ = io.WriteString(w, response)
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cloud := cloudsync.New(store, nil)
	if _, err := cloud.Connect(t.Context(), cloudsync.ConnectRequest{URL: upstream.URL, Token: "test-token"}); err != nil {
		t.Fatal(err)
	}
	handler := (&server.Server{Store: store, Cloud: cloud, Routes: appendCloudRoutes(nil, cloud, nil), AuthKey: "local-key"}).Handler()

	for _, tc := range []struct {
		method, path, body, want string
		status                   int
	}{
		{"GET", "/v1/spaces", "", repos, 200},
		{"POST", "/v1/spaces", `{"name":"Demo","default_git_repos":` + repos + `}`, repos, 201},
		{"POST", "/v1/sessions", `{"backend":"cloud","title":"Inherited","space_id":"project"}`, `"owner_user_id":"owner"`, 200},
		{"POST", "/v1/sessions", `{"backend":"cloud","title":"Selected","space_id":"project","git_repos":` + repos + `}`, `"owner_user_id":"owner"`, 200},
		{"POST", "/v1/sessions", `{"backend":"cloud","title":"Cleared","space_id":"project","git_repos":[]}`, `"owner_user_id":"owner"`, 200},
		{"POST", "/v1/threads/thread/repos", `{"repo":"demo/docs","mount_dir":"guide","base_branch":"release"}`, `"ok":true`, 200},
		{"DELETE", "/v1/threads/thread/repos/guide", "", `"deleted":true`, 200},
		{"GET", "/v1/threads/thread/repo/diff?repo=demo%2Fdocs&path=README.md", "", `"repo":"demo/docs"`, 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer local-key")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.want) {
			t.Errorf("%s %s = %d %s, want %d containing %s", tc.method, tc.path, response.Code, response.Body, tc.status, tc.want)
		}
	}
}
