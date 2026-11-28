package machinebridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBuildConnectURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://cloud.jaz.dev", "wss://cloud.jaz.dev/v1/machines/connect"},
		{"https://cloud.jaz.dev/", "wss://cloud.jaz.dev/v1/machines/connect"},
		{"https://cloud.jaz.dev/api/", "wss://cloud.jaz.dev/api/v1/machines/connect"},
		{"http://127.0.0.1:8080", "ws://127.0.0.1:8080/v1/machines/connect"},
		{"http://localhost:8080", "ws://localhost:8080/v1/machines/connect"},
		{"ws://[::1]:8080", "ws://[::1]:8080/v1/machines/connect"},
		{"wss://cloud.jaz.dev/v1/machines/connect", "wss://cloud.jaz.dev/v1/machines/connect"},
	}
	for _, tc := range cases {
		got, err := BuildConnectURL(tc.in)
		if err != nil {
			t.Fatalf("BuildConnectURL(%q) error = %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("BuildConnectURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{
		"",
		"ftp://example.com",
		"https://",
		"http://cloud.jaz.dev",
		"ws://cloud.jaz.dev/v1/machines/connect",
		"http://10.0.0.1:8080",
	} {
		if _, err := BuildConnectURL(bad); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("BuildConnectURL(%q) err = %v, want ErrInvalidRequest", bad, err)
		}
	}
}

func TestParseGrantSpec(t *testing.T) {
	dir := t.TempDir()
	g, err := ParseGrantSpec("work=rw:" + dir)
	if err != nil {
		t.Fatalf("ParseGrantSpec valid failed: %v", err)
	}
	if g.ID != "work" || g.Mode != AccessReadWrite || g.RootPath != dir {
		t.Fatalf("unexpected grant: %+v", g)
	}

	winGrant, err := ParseGrantSpec(`win=rwd:C:\Users\Alice\Projects`)
	if err != nil {
		t.Fatalf("ParseGrantSpec windows path failed: %v", err)
	}
	if winGrant.ID != "win" || winGrant.Mode != AccessReadWriteDelete || winGrant.RootPath != `C:\Users\Alice\Projects` {
		t.Fatalf("unexpected windows grant: %+v", winGrant)
	}

	for _, bad := range []string{"work", "work=rw", "=rw:/tmp", "work=admin:/tmp", "work=ro:"} {
		if _, err := ParseGrantSpec(bad); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("ParseGrantSpec(%q) err = %v, want ErrInvalidRequest", bad, err)
		}
	}
}

type mockHubConn struct {
	ws      *websocket.Conn
	hello   HelloRequest
	writeMu sync.Mutex
	readMu  sync.Mutex
	pending map[string]chan Frame
	nextID  atomic.Int64
	done    chan struct{}
}

func newMockHubConn(ws *websocket.Conn, hello HelloRequest) *mockHubConn {
	c := &mockHubConn{
		ws:      ws,
		hello:   hello,
		pending: make(map[string]chan Frame),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *mockHubConn) readLoop() {
	defer close(c.done)
	for {
		var frame Frame
		if err := c.ws.ReadJSON(&frame); err != nil {
			return
		}
		c.readMu.Lock()
		ch := c.pending[frame.ID]
		delete(c.pending, frame.ID)
		c.readMu.Unlock()
		if ch != nil {
			ch <- frame
		}
	}
}

func (c *mockHubConn) Call(ctx context.Context, method string, params any) (Frame, error) {
	id := fmt.Sprintf("req-%d", c.nextID.Add(1))
	encoded, err := json.Marshal(params)
	if err != nil {
		return Frame{}, err
	}
	ch := make(chan Frame, 1)
	c.readMu.Lock()
	c.pending[id] = ch
	c.readMu.Unlock()

	c.writeMu.Lock()
	err = c.ws.WriteJSON(Frame{ID: id, Method: method, Params: encoded})
	c.writeMu.Unlock()
	if err != nil {
		return Frame{}, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-c.done:
		return Frame{}, errors.New("hub connection closed")
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

func (c *mockHubConn) Close() error {
	return c.ws.Close()
}

func startMockHub(t *testing.T, wantToken string, serverCaps Capabilities) (*httptest.Server, <-chan *mockHubConn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	conns := make(chan *mockHubConn, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != connectPath {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var helloFrame Frame
		if err := ws.ReadJSON(&helloFrame); err != nil {
			_ = ws.Close()
			return
		}
		if helloFrame.Method != MethodHello {
			_ = ws.Close()
			return
		}
		var helloReq HelloRequest
		if err := json.Unmarshal(helloFrame.Params, &helloReq); err != nil {
			_ = ws.Close()
			return
		}
		helloResp := NegotiateHello(helloReq, CurrentProtocolVersion, serverCaps)
		encoded, _ := json.Marshal(helloResp)
		if err := ws.WriteJSON(Frame{ID: helloFrame.ID, Result: encoded}); err != nil {
			_ = ws.Close()
			return
		}
		if !helloResp.Accepted {
			_ = ws.Close()
			return
		}
		conns <- newMockHubConn(ws, helloReq)
	}))
	t.Cleanup(srv.Close)
	return srv, conns
}

type mockBlobServer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
	headers map[string]http.Header
	puts    atomic.Int64
	gets    atomic.Int64
}

func startMockBlobServer(t *testing.T) *mockBlobServer {
	t.Helper()
	b := &mockBlobServer{
		objects: make(map[string][]byte),
		headers: make(map[string]http.Header),
	}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/")
		switch r.Method {
		case http.MethodPut:
			b.puts.Add(1)
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			b.mu.Lock()
			b.objects[key] = data
			b.headers[key] = r.Header.Clone()
			b.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b.gets.Add(1)
			b.mu.Lock()
			data, ok := b.objects[key]
			b.headers["GET:"+key] = r.Header.Clone()
			b.mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *mockBlobServer) URL(key string) string {
	return b.srv.URL + "/" + key
}

func (b *mockBlobServer) Put(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[key] = append([]byte(nil), data...)
}

func (b *mockBlobServer) Get(key string) ([]byte, http.Header, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.objects[key]
	return append([]byte(nil), data...), b.headers[key].Clone(), ok
}

func TestClientEndToEndOverWSS(t *testing.T) {
	base := t.TempDir()
	outsideDir := filepath.Join(base, "outside")
	roDir := filepath.Join(base, "ro")
	rwDir := filepath.Join(base, "rw")
	rwdDir := filepath.Join(base, "rwd")
	for _, d := range []string{
		outsideDir,
		filepath.Join(roDir, "sub"),
		filepath.Join(roDir, ".git"),
		filepath.Join(roDir, "node_modules", "pkg"),
		rwDir,
		rwdDir,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	outsideSecret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideSecret, []byte("TOP_SECRET_NEEDLE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "hello.txt"), []byte("line one\nfind NEEDLE here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "sub", "needle-doc.txt"), []byte("inside sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "binary.bin"), []byte{0x00, 0x01, 'N', 'E', 'E', 'D', 'L', 'E'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, ".git", "config"), []byte("NEEDLE in git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "node_modules", "pkg", "index.js"), []byte("NEEDLE in node_modules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "too-big.txt"), bytes.Repeat([]byte("a"), 256), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(roDir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("sub", "needle-doc.txt"), filepath.Join(roDir, "safe-link.txt")); err != nil {
		t.Fatal(err)
	}

	rwTarget := filepath.Join(rwDir, "draft.txt")
	rwOriginal := []byte("draft v1\n")
	if err := os.WriteFile(rwTarget, rwOriginal, 0o644); err != nil {
		t.Fatal(err)
	}
	rwdTarget := filepath.Join(rwdDir, "obsolete.txt")
	rwdOriginal := []byte("delete me\n")
	if err := os.WriteFile(rwdTarget, rwdOriginal, 0o644); err != nil {
		t.Fatal(err)
	}

	blobs := startMockBlobServer(t)
	hub, conns := startMockHub(t, "test-secret-token", Capabilities{
		FileStaging:   true,
		LocalExec:     true,
		MaxStageBytes: 128,
	})

	connected := make(chan Capabilities, 1)
	client, err := NewClient(ClientConfig{
		CloudURL:    hub.URL,
		Token:       "test-secret-token",
		DeviceID:    "dev-e2e-1",
		DisplayName: "E2E Laptop",
		Capabilities: Capabilities{
			FileStaging:   true,
			LocalExec:     false,
			MaxStageBytes: 1024,
		},
		FolderGrants: []FolderGrant{
			{ID: "ro", Mode: AccessReadOnly, RootPath: roDir},
			{ID: "rw", Mode: AccessReadWrite, RootPath: rwDir},
			{ID: "rwd", Mode: AccessReadWriteDelete, RootPath: rwdDir},
		},
		AllowLoopbackTransfers: true,
		MinBackoff:             20 * time.Millisecond,
		MaxBackoff:             100 * time.Millisecond,
		OnConnect: func(caps Capabilities) {
			select {
			case connected <- caps:
			default:
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- client.Run(ctx)
	}()

	var conn *mockHubConn
	select {
	case conn = <-conns:
	case <-ctx.Done():
		t.Fatal("timed out waiting for client connection")
	}

	var negotiated Capabilities
	select {
	case negotiated = <-connected:
	case <-ctx.Done():
		t.Fatal("timed out waiting for client OnConnect")
	}
	if !negotiated.FileStaging || negotiated.LocalExec || negotiated.MaxStageBytes != 128 {
		t.Fatalf("unexpected negotiated caps: %+v", negotiated)
	}
	if conn.hello.DeviceID != "dev-e2e-1" || len(conn.hello.FolderGrants) != 3 {
		t.Fatalf("unexpected hello request: %+v", conn.hello)
	}

	// 1. ListDir inside grant skips escaping symlink and enforces MaxEntries.
	resp, err := conn.Call(ctx, MethodListDir, ListDirRequest{GrantID: "ro", Path: ""})
	if err != nil || resp.Error != nil {
		t.Fatalf("ListDir failed: err=%v rpcErr=%v", err, resp.Error)
	}
	var listResp ListDirResponse
	if err := json.Unmarshal(resp.Result, &listResp); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range listResp.Entries {
		names = append(names, e.RelPath)
	}
	if slices.Contains(names, "escape.txt") {
		t.Fatalf("ListDir leaked escaping symlink: %v", names)
	}
	if !slices.Contains(names, "hello.txt") || !slices.Contains(names, "safe-link.txt") || !slices.Contains(names, "sub") {
		t.Fatalf("ListDir missing expected entries: %v", names)
	}

	truncResp, err := conn.Call(ctx, MethodListDir, ListDirRequest{GrantID: "ro", Path: "", MaxEntries: 2})
	if err != nil || truncResp.Error != nil {
		t.Fatalf("ListDir truncated failed: err=%v rpcErr=%v", err, truncResp.Error)
	}
	var truncList ListDirResponse
	if err := json.Unmarshal(truncResp.Result, &truncList); err != nil {
		t.Fatal(err)
	}
	if len(truncList.Entries) != 2 || !truncList.Truncated {
		t.Fatalf("expected 2 truncated entries, got %+v", truncList)
	}

	// ListDir outside grant and on symlink escape denied.
	if r, err := conn.Call(ctx, MethodListDir, ListDirRequest{GrantID: "ro", Path: "../outside"}); err != nil || r.Error == nil || r.Error.Code != CodePathOutsideGrant {
		t.Fatalf("ListDir traversal = (%+v, %v), want %s", r, err, CodePathOutsideGrant)
	}
	if r, err := conn.Call(ctx, MethodListDir, ListDirRequest{GrantID: "ro", Path: "escape.txt"}); err != nil || r.Error == nil || r.Error.Code != CodeSymlinkEscape {
		t.Fatalf("ListDir symlink escape = (%+v, %v), want %s", r, err, CodeSymlinkEscape)
	}

	// 2. Search skips .git, node_modules, binaries, and escaping symlinks.
	searchFrame, err := conn.Call(ctx, MethodSearch, SearchRequest{GrantID: "ro", Query: "needle"})
	if err != nil || searchFrame.Error != nil {
		t.Fatalf("Search failed: err=%v rpcErr=%v", err, searchFrame.Error)
	}
	var searchResp SearchResponse
	if err := json.Unmarshal(searchFrame.Result, &searchResp); err != nil {
		t.Fatal(err)
	}
	if len(searchResp.Matches) != 2 {
		t.Fatalf("Search matches = %+v, want exactly 2 (hello.txt line 2 + sub/needle-doc.txt filename)", searchResp.Matches)
	}
	for _, m := range searchResp.Matches {
		if strings.Contains(m.RelPath, ".git") || strings.Contains(m.RelPath, "node_modules") || m.RelPath == "binary.bin" || m.RelPath == "escape.txt" {
			t.Fatalf("Search returned forbidden match: %+v", m)
		}
	}
	if r, err := conn.Call(ctx, MethodSearch, SearchRequest{GrantID: "ro", Path: "../outside", Query: "secret"}); err != nil || r.Error == nil || r.Error.Code != CodePathOutsideGrant {
		t.Fatalf("Search outside grant = (%+v, %v), want %s", r, err, CodePathOutsideGrant)
	}

	// 3. StageFiles streams directly via HTTP PUT with headers + Content-Type and computes SHA-256.
	stageFrame, err := conn.Call(ctx, MethodStageFiles, DeviceStageRequest{
		GrantID: "ro",
		RunID:   "run-stage-1",
		Files: []StageFileTarget{
			{
				RelPath:       "hello.txt",
				UploadURL:     blobs.URL("staged/hello.txt"),
				UploadHeaders: map[string]string{"X-Upload-Sig": "sig-123"},
				ContentType:   "text/plain; charset=utf-8",
			},
		},
	})
	if err != nil || stageFrame.Error != nil {
		t.Fatalf("StageFiles failed: err=%v rpcErr=%v", err, stageFrame.Error)
	}
	var stageResp DeviceStageResponse
	if err := json.Unmarshal(stageFrame.Result, &stageResp); err != nil {
		t.Fatal(err)
	}
	wantBody := []byte("line one\nfind NEEDLE here\n")
	wantSum := sha256Hex(wantBody)
	if len(stageResp.Files) != 1 || stageResp.Files[0].SHA256 != wantSum || stageResp.Files[0].SizeBytes != int64(len(wantBody)) {
		t.Fatalf("unexpected stage response: %+v", stageResp)
	}
	uploaded, hdr, ok := blobs.Get("staged/hello.txt")
	if !ok || !bytes.Equal(uploaded, wantBody) {
		t.Fatalf("uploaded blob mismatch: ok=%v got=%q", ok, string(uploaded))
	}
	if hdr.Get("X-Upload-Sig") != "sig-123" || hdr.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected upload headers: %+v", hdr)
	}

	// StageFiles rejects file exceeding negotiated MaxStageBytes (128) without calling blob server.
	putsBefore := blobs.puts.Load()
	if r, err := conn.Call(ctx, MethodStageFiles, DeviceStageRequest{
		GrantID: "ro",
		RunID:   "run-stage-big",
		Files:   []StageFileTarget{{RelPath: "too-big.txt", UploadURL: blobs.URL("staged/big.txt")}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeInvalidRequest {
		t.Fatalf("StageFiles oversize = (%+v, %v), want %s", r, err, CodeInvalidRequest)
	}
	if blobs.puts.Load() != putsBefore {
		t.Fatalf("oversize stage unexpectedly performed HTTP PUT")
	}

	// 4. CommitFiles: ro denied, rw conflict on sha256/mtime, rw write succeeds, rw delete denied, rwd delete conflict & success.
	updatedBody := []byte("draft v2 from cloud\n")
	blobs.Put("outputs/draft-v2.txt", updatedBody)
	blobs.Put("outputs/new-nested.txt", []byte("brand new\n"))

	// ro denied
	if r, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "ro",
		RunID:   "run-ro",
		Files:   []CommitFileEntry{{RelPath: "hello.txt", DownloadURL: blobs.URL("outputs/draft-v2.txt")}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeAccessDenied {
		t.Fatalf("ro commit = (%+v, %v), want %s", r, err, CodeAccessDenied)
	}

	// rw conflict on SHA-256 mismatch leaves file untouched.
	if r, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-conflict-sha",
		Files: []CommitFileEntry{{
			RelPath:        "draft.txt",
			DownloadURL:    blobs.URL("outputs/draft-v2.txt"),
			ExpectedSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeConflict {
		t.Fatalf("sha conflict commit = (%+v, %v), want %s", r, err, CodeConflict)
	}

	// rw conflict on mtime mismatch leaves file untouched.
	rwInfo, err := os.Stat(rwTarget)
	if err != nil {
		t.Fatal(err)
	}
	wrongMtime := rwInfo.ModTime().UnixMilli() - 5000
	if r, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-conflict-mtime",
		Files: []CommitFileEntry{{
			RelPath:         "draft.txt",
			DownloadURL:     blobs.URL("outputs/draft-v2.txt"),
			ExpectedMtimeMs: &wrongMtime,
		}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeConflict {
		t.Fatalf("mtime conflict commit = (%+v, %v), want %s", r, err, CodeConflict)
	}
	if disk, _ := os.ReadFile(rwTarget); !bytes.Equal(disk, rwOriginal) {
		t.Fatalf("conflict modified file on disk: %q", string(disk))
	}

	// rw commit with matching precondition + brand-new nested file succeeds atomically.
	exactMtime := rwInfo.ModTime().UnixMilli()
	commitFrame, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-commit-ok",
		Files: []CommitFileEntry{
			{
				RelPath:         "draft.txt",
				DownloadURL:     blobs.URL("outputs/draft-v2.txt"),
				DownloadHeaders: map[string]string{"X-Download-Sig": "dl-1"},
				ExpectedMtimeMs: &exactMtime,
				ExpectedSHA256:  sha256Hex(rwOriginal),
			},
			{
				RelPath:     "nested/new.txt",
				DownloadURL: blobs.URL("outputs/new-nested.txt"),
			},
		},
	})
	if err != nil || commitFrame.Error != nil {
		t.Fatalf("rw commit failed: err=%v rpcErr=%v", err, commitFrame.Error)
	}
	var commitResp DeviceCommitResponse
	if err := json.Unmarshal(commitFrame.Result, &commitResp); err != nil {
		t.Fatal(err)
	}
	if len(commitResp.Files) != 2 || commitResp.Files[0].SHA256 != sha256Hex(updatedBody) || commitResp.Files[0].SizeBytes != int64(len(updatedBody)) {
		t.Fatalf("unexpected commit response: %+v", commitResp)
	}
	if disk, _ := os.ReadFile(rwTarget); !bytes.Equal(disk, updatedBody) {
		t.Fatalf("committed file mismatch: %q", string(disk))
	}
	if disk, _ := os.ReadFile(filepath.Join(rwDir, "nested", "new.txt")); string(disk) != "brand new\n" {
		t.Fatalf("new nested file mismatch: %q", string(disk))
	}

	// rw delete denied.
	if r, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rw",
		RunID:   "run-rw-del",
		Files:   []CommitFileEntry{{RelPath: "draft.txt", Delete: true}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeAccessDenied {
		t.Fatalf("rw delete = (%+v, %v), want %s", r, err, CodeAccessDenied)
	}

	// rwd delete with wrong sha256 -> CodeConflict; matching sha256 -> succeeds.
	if r, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "run-rwd-conflict",
		Files:   []CommitFileEntry{{RelPath: "obsolete.txt", Delete: true, ExpectedSHA256: "deadbeef"}},
	}); err != nil || r.Error == nil || r.Error.Code != CodeConflict {
		t.Fatalf("rwd conflict delete = (%+v, %v), want %s", r, err, CodeConflict)
	}
	delFrame, err := conn.Call(ctx, MethodCommitFiles, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "run-rwd-del",
		Files:   []CommitFileEntry{{RelPath: "obsolete.txt", Delete: true, ExpectedSHA256: sha256Hex(rwdOriginal)}},
	})
	if err != nil || delFrame.Error != nil {
		t.Fatalf("rwd delete failed: err=%v rpcErr=%v", err, delFrame.Error)
	}
	var delResp DeviceCommitResponse
	if err := json.Unmarshal(delFrame.Result, &delResp); err != nil {
		t.Fatal(err)
	}
	if len(delResp.Files) != 1 || !delResp.Files[0].Deleted {
		t.Fatalf("unexpected delete response: %+v", delResp)
	}
	if _, err := os.Stat(rwdTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted file to be gone, stat err = %v", err)
	}

	// 5. ExecStart / ExecStop fail closed when LocalExec is disabled or launcher unset.
	if r, err := conn.Call(ctx, MethodExecStart, ExecStartRequest{GrantID: "rw", RunID: "run-exec"}); err != nil || r.Error == nil || r.Error.Code != CodeUnsupportedCapability {
		t.Fatalf("ExecStart without capability = (%+v, %v), want %s", r, err, CodeUnsupportedCapability)
	}
	if r, err := conn.Call(ctx, MethodExecStop, ExecStopRequest{SessionID: "sess-1"}); err != nil || r.Error == nil || r.Error.Code != CodeUnsupportedCapability {
		t.Fatalf("ExecStop without capability = (%+v, %v), want %s", r, err, CodeUnsupportedCapability)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for client shutdown")
	}
}

type fakeExecLauncher struct {
	mu       sync.Mutex
	started  []string
	stopped  []string
	stopAlls int
}

func (f *fakeExecLauncher) Start(_ context.Context, req ExecStartRequest) (ExecStartResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, req.RunID+":"+req.GrantID)
	return ExecStartResponse{SessionID: "exec-" + req.RunID, Endpoint: "ws://127.0.0.1:9999/acp", AuthToken: "test-token"}, nil
}

func (f *fakeExecLauncher) Stop(_ context.Context, req ExecStopRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, req.SessionID)
	return nil
}

func (f *fakeExecLauncher) StopAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopAlls++
}

func TestClientExecLauncherEnabledOnlyWhenNegotiatedAndConfigured(t *testing.T) {
	dir := t.TempDir()
	roDir := t.TempDir()
	launcher := &fakeExecLauncher{}
	hub, conns := startMockHub(t, "tok", Capabilities{FileStaging: true, LocalExec: true})

	client, err := NewClient(ClientConfig{
		CloudURL:     hub.URL,
		Token:        "tok",
		DeviceID:     "dev-exec",
		Capabilities: Capabilities{FileStaging: true, LocalExec: true},
		FolderGrants: []FolderGrant{
			{ID: "work", Mode: AccessReadWrite, RootPath: dir},
			{ID: "readonly", Mode: AccessReadOnly, RootPath: roDir},
		},
		ExecLauncher: launcher,
		MinBackoff:   20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	conn := <-conns
	// Read-only grant denied for ExecStart.
	if roFrame, err := conn.Call(ctx, MethodExecStart, ExecStartRequest{GrantID: "readonly", RunID: "r-ro"}); err != nil || roFrame.Error == nil || roFrame.Error.Code != CodeAccessDenied {
		t.Fatalf("ro ExecStart = (%+v, %v), want %s", roFrame, err, CodeAccessDenied)
	}

	startFrame, err := conn.Call(ctx, MethodExecStart, ExecStartRequest{GrantID: "work", RunID: "r1"})
	if err != nil || startFrame.Error != nil {
		t.Fatalf("ExecStart failed: err=%v rpcErr=%v", err, startFrame.Error)
	}
	var startResp ExecStartResponse
	if err := json.Unmarshal(startFrame.Result, &startResp); err != nil {
		t.Fatal(err)
	}
	if startResp.SessionID != "exec-r1" || startResp.AuthToken != "test-token" {
		t.Fatalf("unexpected ExecStartResponse: %+v", startResp)
	}
	stopFrame, err := conn.Call(ctx, MethodExecStop, ExecStopRequest{SessionID: "exec-r1"})
	if err != nil || stopFrame.Error != nil {
		t.Fatalf("ExecStop failed: err=%v rpcErr=%v", err, stopFrame.Error)
	}
	// Closing connection triggers StopAll on disconnect.
	_ = conn.Close()
	<-conns
	launcher.mu.Lock()
	stopAlls := launcher.stopAlls
	launcher.mu.Unlock()
	if stopAlls < 1 {
		t.Fatalf("expected StopAll on disconnect, got %d", stopAlls)
	}
	cancel()
	<-done
}

func TestHandlerTransferRedirectsAndTwoPhaseCommitAtomicity(t *testing.T) {
	dir := t.TempDir()
	keepFile := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keepFile, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(targetFile, []byte("keep-real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	var redirectHits atomic.Int64
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHits.Add(1)
		_, _ = w.Write([]byte("redirected-secret"))
	}))
	defer redirectTarget.Close()

	blobs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, redirectTarget.URL+"/secret", http.StatusFound)
		case "/ok":
			_, _ = w.Write([]byte("new-v2\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer blobs.Close()

	// Default production guard blocks loopback transfers.
	prodGuard, err := NewPolicyGuard([]FolderGrant{{ID: "rwd", Mode: AccessReadWriteDelete, RootPath: dir}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prodGuard.Close() })
	prodHandler := NewHandler(prodGuard, nil, nil)
	if _, err := prodHandler.StageFiles(t.Context(), Capabilities{FileStaging: true}, DeviceStageRequest{
		GrantID: "rwd",
		RunID:   "r-prod",
		Files:   []StageFileTarget{{RelPath: "keep.txt", UploadURL: blobs.URL + "/ok"}},
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("prod StageFiles loopback err = %v, want ErrInvalidRequest", err)
	}

	guard, err := NewPolicyGuard([]FolderGrant{{ID: "rwd", Mode: AccessReadWriteDelete, RootPath: dir}}, WithAllowLoopbackTransfers())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	handler := NewHandler(guard, nil, nil)
	caps := Capabilities{FileStaging: true}

	// Redirect is not followed on download or upload.
	if _, err := handler.CommitFiles(t.Context(), caps, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "r-redir",
		Files:   []CommitFileEntry{{RelPath: "keep.txt", DownloadURL: blobs.URL + "/redirect"}},
	}); err == nil {
		t.Fatal("expected redirect download to fail")
	}
	if _, err := handler.StageFiles(t.Context(), caps, DeviceStageRequest{
		GrantID: "rwd",
		RunID:   "r-redir-put",
		Files:   []StageFileTarget{{RelPath: "keep.txt", UploadURL: blobs.URL + "/redirect"}},
	}); err == nil {
		t.Fatal("expected redirect upload to fail")
	}
	if redirectHits.Load() != 0 {
		t.Fatalf("redirect target was contacted %d times, want 0", redirectHits.Load())
	}

	// Batch where second download fails must not mutate first file or leave temp files.
	if _, err := handler.CommitFiles(t.Context(), caps, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "r-partial",
		Files: []CommitFileEntry{
			{RelPath: "keep.txt", DownloadURL: blobs.URL + "/ok"},
			{RelPath: "missing.txt", DownloadURL: blobs.URL + "/does-not-exist"},
		},
	}); err == nil {
		t.Fatal("expected failed second download to abort commit")
	}
	if got, _ := os.ReadFile(keepFile); string(got) != "original\n" {
		t.Fatalf("failed batch partially modified keep.txt: %q", string(got))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".jaz-commit-") {
			t.Fatalf("failed batch left temp file %q", entry.Name())
		}
	}

	// Symlink delete attempt must be rejected without touching target or link.
	if _, err := handler.CommitFiles(t.Context(), caps, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "r-symlink-del",
		Files:   []CommitFileEntry{{RelPath: "link.txt", Delete: true}},
	}); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("symlink delete err = %v, want ErrSymlinkEscape", err)
	}
	if got, err := os.ReadFile(targetFile); err != nil || string(got) != "keep-real\n" {
		t.Fatalf("symlink delete touched target: (%q, %v)", string(got), err)
	}

	// Client auto-defaults AllowedTransferHosts from CloudURL host + storage.googleapis.com and rejects arbitrary HTTPS hosts.
	cloudClient, err := NewClient(ClientConfig{
		CloudURL:     "https://cloud.jaz.dev",
		Token:        "tok",
		DeviceID:     "dev-pin",
		FolderGrants: []FolderGrant{{ID: "rw", Mode: AccessReadWrite, RootPath: dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cloudClient.Close() })
	if _, err := cloudClient.guard.ValidateStageRequest(DeviceStageRequest{
		GrantID: "rw",
		RunID:   "r-pin-ok",
		Files: []StageFileTarget{
			{RelPath: "keep.txt", UploadURL: "https://cloud.jaz.dev/v1/blobs/up"},
			{RelPath: "keep.txt", UploadURL: "https://storage.googleapis.com/jaz-bucket/up"},
		},
	}); err != nil {
		t.Fatalf("default CloudURL/GCS transfer hosts rejected: %v", err)
	}
	if _, err := cloudClient.guard.ValidateStageRequest(DeviceStageRequest{
		GrantID: "rw",
		RunID:   "r-pin-evil",
		Files:   []StageFileTarget{{RelPath: "keep.txt", UploadURL: "https://evil-exfil.example/steal"}},
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("third-party HTTPS exfil host err = %v, want ErrInvalidRequest", err)
	}

	// Fallback download cap (defaultMaxStageBytes = 100 MiB) rejects unbounded downloads when MaxStageBytes == 0.
	hugeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(zeroReader{}, defaultMaxStageBytes+2))
	}))
	defer hugeServer.Close()
	if _, err := handler.CommitFiles(t.Context(), Capabilities{FileStaging: true, MaxStageBytes: 0}, DeviceCommitRequest{
		GrantID: "rwd",
		RunID:   "r-huge-fallback",
		Files:   []CommitFileEntry{{RelPath: "huge.bin", DownloadURL: hugeServer.URL + "/huge"}},
	}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversize fallback download err = %v, want ErrInvalidRequest", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "huge.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversize fallback download wrote huge.bin: err=%v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestClientReconnectsAfterServerDrop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ping.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	hub, conns := startMockHub(t, "reconnect-tok", Capabilities{FileStaging: true})

	client, err := NewClient(ClientConfig{
		CloudURL:     hub.URL,
		Token:        "reconnect-tok",
		DeviceID:     "dev-reconn",
		Capabilities: Capabilities{FileStaging: true},
		FolderGrants: []FolderGrant{{ID: "work", Mode: AccessReadOnly, RootPath: dir}},
		MinBackoff:   20 * time.Millisecond,
		MaxBackoff:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	first := <-conns
	if r, err := first.Call(ctx, MethodListDir, ListDirRequest{GrantID: "work"}); err != nil || r.Error != nil {
		t.Fatalf("first ListDir failed: err=%v rpcErr=%v", err, r.Error)
	}
	_ = first.Close()

	var second *mockHubConn
	select {
	case second = <-conns:
	case <-ctx.Done():
		t.Fatal("client did not reconnect after server drop")
	}

	// Verify concurrent RPC dispatch on the reconnected session.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := second.Call(ctx, MethodListDir, ListDirRequest{GrantID: "work"})
			if err != nil {
				errs <- err
				return
			}
			if r.Error != nil {
				errs <- r.Error
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent RPC after reconnect failed: %v", err)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
