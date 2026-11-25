package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	assetGrantTTL          = 5 * time.Minute
	assetGrantPurpose      = "jaz-asset-grant-v1:"
	assetGrantPrefix       = "ag_"
	assetGrantVersion      = 1
	maxAssetGrantBodyBytes = 8192
)

var assetExtensionTypes = map[string]string{
	".glb":     "model/gltf-binary",
	".gltf":    "model/gltf+json",
	".stl":     "model/stl",
	".obj":     "model/obj",
	".ifc":     "application/x-step",
	".step":    "application/step",
	".stp":     "application/step",
	".parquet": "application/vnd.apache.parquet",
	".ktx2":    "image/ktx2",
	".wasm":    "application/wasm",
	".pdf":     "application/pdf",
}

type assetGrantClaims struct {
	SessionID string `json:"sid"`
	RelPath   string `json:"rel"`
	ExpiresAt int64  `json:"exp"`
	Version   int    `json:"ver"`
}

type assetGrantResponse struct {
	URL         string    `json:"url"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	ETag        string    `json:"etag,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Server) assetGrantKey() []byte {
	if s.AuthKey != "" {
		return []byte(s.AuthKey)
	}
	s.assetKeyOnce.Do(func() {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("crypto/rand failed for asset grant key: " + err.Error())
		}
		s.assetKey = key
	})
	return s.assetKey
}

func issueAssetGrant(key []byte, sessionID, relPath string, now time.Time) (string, time.Time, error) {
	if len(key) == 0 {
		return "", time.Time{}, fmt.Errorf("asset grant signing key is unavailable")
	}
	expires := now.UTC().Add(assetGrantTTL).Truncate(time.Second)
	raw, err := json.Marshal(assetGrantClaims{
		SessionID: sessionID,
		RelPath:   relPath,
		ExpiresAt: expires.Unix(),
		Version:   assetGrantVersion,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sig := signAssetGrant(key, payload)
	return assetGrantPrefix + payload + "." + sig, expires, nil
}

func parseAssetGrant(key []byte, token string, now time.Time) (assetGrantClaims, bool) {
	if len(key) == 0 {
		return assetGrantClaims{}, false
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(token), assetGrantPrefix)
	if !ok {
		return assetGrantClaims{}, false
	}
	payload, sig, ok := strings.Cut(rest, ".")
	if !ok || payload == "" || sig == "" {
		return assetGrantClaims{}, false
	}
	expected := signAssetGrant(key, payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return assetGrantClaims{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return assetGrantClaims{}, false
	}
	var claims assetGrantClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return assetGrantClaims{}, false
	}
	if claims.Version != assetGrantVersion || claims.SessionID == "" || claims.RelPath == "" {
		return assetGrantClaims{}, false
	}
	if !now.UTC().Before(time.Unix(claims.ExpiresAt, 0).UTC()) {
		return assetGrantClaims{}, false
	}
	return claims, true
}

func signAssetGrant(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(assetGrantPurpose))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func cleanAssetRelPath(raw string) (string, error) {
	if raw == "" || strings.ContainsRune(raw, 0) || strings.ContainsRune(raw, '\\') {
		return "", fmt.Errorf("invalid asset path")
	}
	if strings.HasPrefix(raw, "/") || filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", fmt.Errorf("asset path must be relative to the session working directory")
	}
	parts := strings.Split(raw, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("asset path must stay inside the session working directory")
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("asset path is required")
	}
	rel := strings.Join(clean, "/")
	if !fs.ValidPath(rel) {
		return "", fmt.Errorf("invalid asset path")
	}
	return rel, nil
}

func encodeAssetRelPath(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func parseSessionAssetPath(urlPath string) (string, string, bool) {
	rest, ok := strings.CutPrefix(urlPath, "/v1/sessions/")
	if !ok {
		return "", "", false
	}
	sessionRef, action, ok := strings.Cut(rest, "/")
	if !ok || sessionRef == "" {
		return "", "", false
	}
	rawRel, ok := strings.CutPrefix(action, "assets/")
	if !ok || rawRel == "" || strings.HasPrefix(rawRel, "/") || strings.Contains(rawRel, "//") {
		return "", "", false
	}
	rel, err := cleanAssetRelPath(rawRel)
	if err != nil {
		return "", "", false
	}
	return sessionRef, rel, true
}

func hasEncodedPathSeparator(r *http.Request) bool {
	if r.URL.RawPath == "" {
		return false
	}
	lower := strings.ToLower(r.URL.RawPath)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c")
}

func sessionAssetContentRequest(r *http.Request) bool {
	_, _, ok := parseSessionAssetPath(r.URL.Path)
	return ok && (r.Method == http.MethodGet || r.Method == http.MethodHead)
}

func isSessionAssetGrantFetch(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if hasEncodedPathSeparator(r) {
		return false
	}
	_, _, ok := parseSessionAssetPath(r.URL.Path)
	return ok && strings.TrimSpace(r.URL.Query().Get("grant")) != ""
}

func isSessionAssetGrantCORSRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		return false
	}
	if r.Header.Get("Authorization") != "" || hasEncodedPathSeparator(r) {
		return false
	}
	_, _, ok := parseSessionAssetPath(r.URL.Path)
	return ok && strings.TrimSpace(r.URL.Query().Get("grant")) != ""
}

func openConfinedSessionAsset(session storage.Session, rel string) (*os.File, os.FileInfo, int, error) {
	cwd := optionalCwd(session)
	if cwd == "" {
		return nil, nil, http.StatusBadRequest, fmt.Errorf("session has no working directory")
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, nil, http.StatusNotFound, fmt.Errorf("session working directory is unavailable")
	}
	defer root.Close()
	file, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, http.StatusNotFound, fmt.Errorf("asset not found: %s", rel)
		}
		return nil, nil, http.StatusForbidden, fmt.Errorf("asset path escapes the session working directory")
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, http.StatusNotFound, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, http.StatusBadRequest, fmt.Errorf("asset path is not a regular file: %s", rel)
	}
	return file, info, http.StatusOK, nil
}

func assetContentType(rel string) string {
	ext := strings.ToLower(filepath.Ext(rel))
	if ctype, ok := assetExtensionTypes[ext]; ok {
		return ctype
	}
	if ctype := mime.TypeByExtension(ext); ctype != "" {
		return ctype
	}
	return "application/octet-stream"
}

func assetDispositionType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "attachment"
	}
	switch {
	case mediaType == "application/pdf",
		mediaType == "image/png",
		mediaType == "image/jpeg",
		mediaType == "image/webp",
		mediaType == "image/gif",
		mediaType == "image/avif",
		strings.HasPrefix(mediaType, "model/"),
		strings.HasPrefix(mediaType, "audio/"),
		strings.HasPrefix(mediaType, "video/"):
		return "inline"
	default:
		return "attachment"
	}
}

func (s *Server) handleGrantSessionAsset(w http.ResponseWriter, r *http.Request) {
	sessionRef := r.PathValue("session")
	session, err := s.Store.LoadSession(sessionRef)
	if err == nil && storage.NormalizeWorkspaceID(session.WorkspaceID) != s.activeWorkspaceID() {
		err = storage.ErrSessionNotFound
	}
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAssetGrantBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid asset grant request"))
		return
	}
	rel, err := cleanAssetRelPath(strings.TrimSpace(req.Path))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file, info, status, err := openConfinedSessionAsset(session, rel)
	if err != nil {
		writeError(w, status, err)
		return
	}
	file.Close()
	token, expiresAt, err := issueAssetGrant(s.assetGrantKey(), session.ID, rel, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	assetPath := fmt.Sprintf("/v1/sessions/%s/assets/%s?grant=%s",
		url.PathEscape(session.ID),
		encodeAssetRelPath(rel),
		url.QueryEscape(token),
	)
	assetURL := assetPath
	if base := httpapi.RequestBaseURL(r); base != "" {
		assetURL = base + assetPath
	}
	writeJSON(w, http.StatusOK, assetGrantResponse{
		URL:         assetURL,
		Size:        info.Size(),
		ContentType: assetContentType(rel),
		ETag:        fileETag(info),
		ExpiresAt:   expiresAt,
	})
}

func (s *Server) handleGetSessionAsset(w http.ResponseWriter, r *http.Request) {
	if hasEncodedPathSeparator(r) {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("invalid asset grant"))
		return
	}
	rel, err := cleanAssetRelPath(r.PathValue("rel"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	grant := strings.TrimSpace(r.URL.Query().Get("grant"))
	claims, ok := parseAssetGrant(s.assetGrantKey(), grant, time.Now().UTC())
	if !ok || claims.RelPath != rel {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("invalid or expired asset grant"))
		return
	}
	session, err := s.Store.LoadSession(r.PathValue("session"))
	if err == nil && storage.NormalizeWorkspaceID(session.WorkspaceID) != s.activeWorkspaceID() {
		err = storage.ErrSessionNotFound
	}
	if err != nil || session.ID != claims.SessionID {
		writeAuthError(w, http.StatusUnauthorized, "unauthorized", fmt.Errorf("invalid or expired asset grant"))
		return
	}
	file, info, status, err := openConfinedSessionAsset(session, rel)
	if err != nil {
		writeError(w, status, err)
		return
	}
	defer file.Close()
	contentType := assetContentType(rel)
	w.Header().Set("Content-Type", contentType)
	if disposition := mime.FormatMediaType(assetDispositionType(contentType), map[string]string{"filename": filepath.Base(rel)}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("ETag", fileETag(info))
	http.ServeContent(w, r, filepath.Base(rel), info.ModTime(), file)
}
