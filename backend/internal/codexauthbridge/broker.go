package codexauthbridge

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/wins/jaz/backend/internal/acpcredentials"
)

const (
	brokerURLEnv        = "JAZ_CODEX_PROFILE_URL"
	brokerTokenEnv      = "JAZ_CODEX_PROFILE_TOKEN"
	nativeExecutableEnv = "JAZ_CODEX_PROFILE_EXECUTABLE"
)

func Install(ctx context.Context, env map[string]string, executable string, resolve func(context.Context, int64) (acpcredentials.Profile, error)) (func(), error) {
	proxy, err := os.Executable()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := rand.Text()
	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
			if err != nil || version < 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			profile, err := resolve(r.Context(), version)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(profile)
		}),
	}
	go func() { _ = srv.Serve(listener) }()
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	env["CODEX_PATH"] = proxy
	env[brokerURLEnv] = "http://" + listener.Addr().String()
	env[brokerTokenEnv] = token
	env[nativeExecutableEnv] = executable
	return func() {
		stop()
		_ = listener.Close()
		_ = srv.Close()
	}, nil
}

func fetchProfile(ctx context.Context, version int64) (acpcredentials.Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv(brokerURLEnv)+"?version="+strconv.FormatInt(version, 10), nil)
	if err != nil || req.URL.Hostname() != "127.0.0.1" {
		return acpcredentials.Profile{}, errors.New("invalid Codex credentials bridge")
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv(brokerTokenEnv))
	client := &http.Client{
		Timeout:       35 * time.Second,
		Transport:     &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return acpcredentials.Profile{}, errors.New("Codex credentials bridge unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return acpcredentials.Profile{}, errors.New("could not resolve Codex workspace credentials")
	}
	var profile acpcredentials.Profile
	if json.NewDecoder(resp.Body).Decode(&profile) != nil || profile.ChatGPT == nil {
		return acpcredentials.Profile{}, errors.New("invalid Codex credentials")
	}
	return profile, nil
}
