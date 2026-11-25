package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionAssetGrantAndRangeStreaming(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "outputs"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("0123456789abcdef"), 128)
	modelPath := filepath.Join(dir, "outputs", "my model #1.glb")
	if err := os.WriteFile(modelPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(dir, "outputs", "other.glb")
	if err := os.WriteFile(otherPath, []byte("other-binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(dir, "outputs", "note.html")
	if err := os.WriteFile(htmlPath, []byte("<script>alert(1)</script>"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("top-secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(dir, "outputs", "escape-link.glb")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape-dir")); err != nil {
		t.Fatal(err)
	}

	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessionA, err := store.CreateSession(storage.CreateSession{
		Slug:       "asset-session-a",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := store.CreateSession(storage.CreateSession{
		Slug:       "asset-session-b",
		RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Cwd: dir},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: store, AuthKey: "test-secret"}
	handler := srv.Handler()

	mint := func(sessionID, path, auth string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"path": path})
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5299/v1/sessions/"+sessionID+"/assets/grant", bytes.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// Unauthenticated and resource-ticket minting must fail.
	if res := mint(sessionA.ID, "outputs/my model #1.glb", ""); res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated grant status = %d, want 401", res.Code)
	}
	ticket := issueResourceTicket(srv.AuthKey, time.Now().UTC())
	reqTicketMint := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionA.ID+"/assets/grant?ticket="+url.QueryEscape(ticket.Token), strings.NewReader(`{"path":"outputs/my model #1.glb"}`))
	resTicketMint := httptest.NewRecorder()
	handler.ServeHTTP(resTicketMint, reqTicketMint)
	if resTicketMint.Code != http.StatusUnauthorized {
		t.Fatalf("resource-ticket grant mint status = %d, want 401", resTicketMint.Code)
	}

	// Path traversal, absolute paths, and symlink escapes must all be rejected.
	for _, bad := range []string{
		"",
		".",
		"..",
		"../secret.txt",
		"outputs/../outputs/other.glb",
		"outputs/./other.glb",
		"outputs//other.glb",
		"outputs/",
		"C:outputs/other.glb",
		outsideFile,
		"/etc/passwd",
		`outputs\other.glb`,
		"outputs/other.glb\x00",
		"outputs",
		"outputs/escape-link.glb",
		"escape-dir/secret.txt",
	} {
		res := mint(sessionA.ID, bad, "Bearer test-secret")
		if res.Code == http.StatusOK {
			t.Fatalf("grant unexpectedly accepted %q: %s", bad, res.Body.String())
		}
	}

	// Valid grant mint.
	granted := mint(sessionA.ID, "outputs/my model #1.glb", "Bearer test-secret")
	if granted.Code != http.StatusOK {
		t.Fatalf("grant status = %d, body = %s", granted.Code, granted.Body.String())
	}
	var grantResp assetGrantResponse
	if err := json.Unmarshal(granted.Body.Bytes(), &grantResp); err != nil {
		t.Fatal(err)
	}
	if grantResp.Size != int64(len(payload)) || grantResp.ContentType != "model/gltf-binary" || grantResp.ETag == "" {
		t.Fatalf("grant metadata = %+v", grantResp)
	}
	if ttl := time.Until(grantResp.ExpiresAt); ttl < 4*time.Minute || ttl > 5*time.Minute+5*time.Second {
		t.Fatalf("grant TTL = %v, want ~5m", ttl)
	}
	parsedURL, err := url.Parse(grantResp.URL)
	if err != nil {
		t.Fatalf("invalid grant URL %q: %v", grantResp.URL, err)
	}
	grantToken := parsedURL.Query().Get("grant")
	if !strings.HasPrefix(grantToken, "ag_") {
		t.Fatalf("grant token = %q, want ag_ prefix", grantToken)
	}
	if validResourceTicket(srv.AuthKey, grantToken, time.Now().UTC()) {
		t.Fatal("asset grant token must not validate as a resource ticket")
	}

	// Tier 1 CORS preflight on ?grant= from Origin: null.
	preflight := httptest.NewRequest(http.MethodOptions, parsedURL.RequestURI(), nil)
	preflight.Header.Set("Origin", "null")
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflight.Header.Set("Access-Control-Request-Headers", "Range, If-Range")
	preflightRes := httptest.NewRecorder()
	handler.ServeHTTP(preflightRes, preflight)
	if preflightRes.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, body = %s", preflightRes.Code, preflightRes.Body.String())
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("ACAO = %q, want *", got)
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("ACAC = %q, want empty", got)
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("ACAPN = %q, want true", got)
	}
	if got := preflightRes.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Range") || !strings.Contains(got, "If-Range") {
		t.Fatalf("ACAH = %q", got)
	}
	if got := preflightRes.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Content-Range") || !strings.Contains(got, "Accept-Ranges") || !strings.Contains(got, "Content-Length") || !strings.Contains(got, "Content-Type") || !strings.Contains(got, "ETag") {
		t.Fatalf("ACEH = %q", got)
	}

	// Non-grant requests from Origin: null must stay blocked by CORS.
	noGrantNull := httptest.NewRequest(http.MethodGet, parsedURL.EscapedPath(), nil)
	noGrantNull.Header.Set("Origin", "null")
	noGrantNullRes := httptest.NewRecorder()
	handler.ServeHTTP(noGrantNullRes, noGrantNull)
	if noGrantNullRes.Code != http.StatusForbidden {
		t.Fatalf("Origin: null without ?grant= status = %d, want 403", noGrantNullRes.Code)
	}

	// GET with Range + Accept-Encoding: gzip must return 206 uncompressed with Tier 1 CORS headers.
	rangeReq := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	rangeReq.Header.Set("Origin", "null")
	rangeReq.Header.Set("Range", "bytes=10-25")
	rangeReq.Header.Set("Accept-Encoding", "gzip")
	rangeRes := httptest.NewRecorder()
	handler.ServeHTTP(rangeRes, rangeReq)
	if rangeRes.Code != http.StatusPartialContent {
		t.Fatalf("range GET status = %d, want 206; body = %s", rangeRes.Code, rangeRes.Body.String())
	}
	if !bytes.Equal(rangeRes.Body.Bytes(), payload[10:26]) {
		t.Fatalf("range body = %q, want %q", rangeRes.Body.Bytes(), payload[10:26])
	}
	if got := rangeRes.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want gzip bypassed", got)
	}
	if got := rangeRes.Header().Get("Content-Range"); got != "bytes 10-25/2048" {
		t.Fatalf("Content-Range = %q, want bytes 10-25/2048", got)
	}
	if got := rangeRes.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rangeRes.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rangeRes.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("GET ACAO = %q, want *", got)
	}

	// Non-Range GET with Accept-Encoding: gzip must also bypass gzip.
	fullReq := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	fullReq.Header.Set("Accept-Encoding", "gzip")
	fullRes := httptest.NewRecorder()
	handler.ServeHTTP(fullRes, fullReq)
	if fullRes.Code != http.StatusOK || fullRes.Header().Get("Content-Encoding") != "" || !bytes.Equal(fullRes.Body.Bytes(), payload) {
		t.Fatalf("full GET status=%d encoding=%q len=%d", fullRes.Code, fullRes.Header().Get("Content-Encoding"), fullRes.Body.Len())
	}

	// HEAD must return metadata with empty body.
	headReq := httptest.NewRequest(http.MethodHead, parsedURL.RequestURI(), nil)
	headReq.Header.Set("Origin", "null")
	headRes := httptest.NewRecorder()
	handler.ServeHTTP(headRes, headReq)
	if headRes.Code != http.StatusOK || headRes.Body.Len() != 0 || headRes.Header().Get("Accept-Ranges") != "bytes" || headRes.Header().Get("Content-Length") != "2048" {
		t.Fatalf("HEAD status=%d body=%d accept-ranges=%q content-length=%q", headRes.Code, headRes.Body.Len(), headRes.Header().Get("Accept-Ranges"), headRes.Header().Get("Content-Length"))
	}

	// HTML assets must use attachment disposition + sandbox CSP.
	htmlGrantRes := mint(sessionA.ID, "outputs/note.html", "Bearer test-secret")
	if htmlGrantRes.Code != http.StatusOK {
		t.Fatalf("html grant status = %d", htmlGrantRes.Code)
	}
	var htmlGrant assetGrantResponse
	if err := json.Unmarshal(htmlGrantRes.Body.Bytes(), &htmlGrant); err != nil {
		t.Fatal(err)
	}
	htmlURL, _ := url.Parse(htmlGrant.URL)
	htmlFetch := httptest.NewRecorder()
	handler.ServeHTTP(htmlFetch, httptest.NewRequest(http.MethodGet, htmlURL.RequestURI(), nil))
	if !strings.HasPrefix(htmlFetch.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("html Content-Disposition = %q, want attachment", htmlFetch.Header().Get("Content-Disposition"))
	}

	// Single-file & single-session scoping + global ticket rejection + expiry.
	wrongFile := httptest.NewRecorder()
	handler.ServeHTTP(wrongFile, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionA.ID+"/assets/outputs/other.glb?grant="+url.QueryEscape(grantToken), nil))
	if wrongFile.Code != http.StatusUnauthorized {
		t.Fatalf("wrong file with grant status = %d, want 401", wrongFile.Code)
	}

	wrongSession := httptest.NewRecorder()
	handler.ServeHTTP(wrongSession, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionB.ID+"/assets/outputs/my%20model%20%231.glb?grant="+url.QueryEscape(grantToken), nil))
	if wrongSession.Code != http.StatusUnauthorized {
		t.Fatalf("wrong session with grant status = %d, want 401", wrongSession.Code)
	}

	resourceTicketOnAsset := httptest.NewRecorder()
	handler.ServeHTTP(resourceTicketOnAsset, httptest.NewRequest(http.MethodGet, parsedURL.EscapedPath()+"?ticket="+url.QueryEscape(ticket.Token), nil))
	if resourceTicketOnAsset.Code != http.StatusUnauthorized {
		t.Fatalf("resource ticket on asset route status = %d, want 401", resourceTicketOnAsset.Code)
	}

	grantAsResourceTicket := httptest.NewRecorder()
	handler.ServeHTTP(grantAsResourceTicket, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionA.ID+"/file?path=outputs/other.glb&raw=1&ticket="+url.QueryEscape(grantToken), nil))
	if grantAsResourceTicket.Code != http.StatusUnauthorized {
		t.Fatalf("asset grant used as ?ticket= status = %d, want 401", grantAsResourceTicket.Code)
	}

	expiredToken, _, err := issueAssetGrant(srv.assetGrantKey(), sessionA.ID, "outputs/other.glb", time.Now().UTC().Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	expiredRes := httptest.NewRecorder()
	handler.ServeHTTP(expiredRes, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionA.ID+"/assets/outputs/other.glb?grant="+url.QueryEscape(expiredToken), nil))
	if expiredRes.Code != http.StatusUnauthorized {
		t.Fatalf("expired grant status = %d, want 401", expiredRes.Code)
	}

	// Bearer-authed requests must NEVER get Tier 1 CORS or read assets without a valid grant.
	bearerAsset := httptest.NewRequest(http.MethodGet, parsedURL.EscapedPath(), nil)
	bearerAsset.Header.Set("Authorization", "Bearer test-secret")
	bearerAssetRes := httptest.NewRecorder()
	handler.ServeHTTP(bearerAssetRes, bearerAsset)
	if bearerAssetRes.Code != http.StatusUnauthorized || bearerAssetRes.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatalf("bearer-only asset fetch status=%d acao=%q, want 401 and no *", bearerAssetRes.Code, bearerAssetRes.Header().Get("Access-Control-Allow-Origin"))
	}

	mixedAuth := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	mixedAuth.Header.Set("Origin", "null")
	mixedAuth.Header.Set("Authorization", "Bearer test-secret")
	mixedAuthRes := httptest.NewRecorder()
	handler.ServeHTTP(mixedAuthRes, mixedAuth)
	if mixedAuthRes.Code != http.StatusForbidden {
		t.Fatalf("Origin: null with Authorization + ?grant= status = %d, want 403", mixedAuthRes.Code)
	}

	ticketAsGrantNull := httptest.NewRequest(http.MethodGet, parsedURL.EscapedPath()+"?grant="+url.QueryEscape(ticket.Token), nil)
	ticketAsGrantNull.Header.Set("Origin", "null")
	ticketAsGrantNullRes := httptest.NewRecorder()
	handler.ServeHTTP(ticketAsGrantNullRes, ticketAsGrantNull)
	if ticketAsGrantNullRes.Code != http.StatusForbidden {
		t.Fatalf("Origin: null with ?grant=tk_... status = %d, want 403", ticketAsGrantNullRes.Code)
	}

	// Post-grant symlink swap (TOCTOU protection) must still be rejected by os.OpenRoot.
	swapPath := filepath.Join(dir, "outputs", "swappable.glb")
	if err := os.WriteFile(swapPath, []byte("legit"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapGrantRes := mint(sessionA.ID, "outputs/swappable.glb", "Bearer test-secret")
	if swapGrantRes.Code != http.StatusOK {
		t.Fatalf("swap grant status = %d", swapGrantRes.Code)
	}
	var swapGrant assetGrantResponse
	if err := json.Unmarshal(swapGrantRes.Body.Bytes(), &swapGrant); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(swapPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, swapPath); err != nil {
		t.Fatal(err)
	}
	swapURL, _ := url.Parse(swapGrant.URL)
	swapFetch := httptest.NewRecorder()
	handler.ServeHTTP(swapFetch, httptest.NewRequest(http.MethodGet, swapURL.RequestURI(), nil))
	if swapFetch.Code != http.StatusForbidden {
		t.Fatalf("post-grant symlink swap status = %d, want 403; body = %s", swapFetch.Code, swapFetch.Body.String())
	}

	// Even when AuthKey is empty, GET /assets/{rel} must require a valid signed ag_ grant.
	noAuthSrv := &Server{Store: store}
	noAuthHandler := noAuthSrv.Handler()
	noAuthUnauth := httptest.NewRecorder()
	noAuthHandler.ServeHTTP(noAuthUnauth, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionA.ID+"/assets/outputs/other.glb", nil))
	if noAuthUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth server without grant status = %d, want 401", noAuthUnauth.Code)
	}
	noAuthForged := httptest.NewRecorder()
	noAuthHandler.ServeHTTP(noAuthForged, httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionA.ID+"/assets/outputs/other.glb?grant="+url.QueryEscape(grantToken), nil))
	if noAuthForged.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth server with foreign grant status = %d, want 401", noAuthForged.Code)
	}
}
