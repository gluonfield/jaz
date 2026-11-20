package mcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func (r *callbackReceiver) redirectURL(ctx context.Context, callbackURL string) (string, error) {
	if !r.pending.loopback {
		return callbackURL, nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen for local OAuth callback: %w", err)
	}
	address := listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+mcpconfig.OAuthCallbackPath, func(w http.ResponseWriter, req *http.Request) {
		if req.Host != address || req.Method != http.MethodGet {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q := req.URL.Query()
		response := mcpconfig.AuthorizationResponse{State: q.Get("state"), Code: q.Get("code"), Issuer: q.Get("iss"), Error: q.Get("error")}
		if err := r.manager.completeAuthorization(req.Context(), response, r.pending); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Connected to Jaz</title><p>Connected. You can close this tab and return to Jaz.</p></html>`)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	context.AfterFunc(ctx, func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	})
	go func() {
		_ = server.Serve(listener)
	}()
	return "http://" + address + mcpconfig.OAuthCallbackPath, nil
}
