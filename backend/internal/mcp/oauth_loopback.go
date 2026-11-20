package mcp

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/wins/jaz/backend/internal/mcpconfig"
)

func (r *callbackReceiver) serveLoopback(ctx context.Context, state string) {
	address := r.loopback.Addr().String()
	var received atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host != address || req.Method != http.MethodGet || req.URL.Path != mcpconfig.OAuthCallbackPath {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q := req.URL.Query()
		if q.Get("state") != state || ctx.Err() != nil || !received.CompareAndSwap(false, true) {
			http.Error(w, "Invalid or expired authorization state", http.StatusBadRequest)
			return
		}
		_ = r.loopback.Close()
		response := mcpconfig.AuthorizationResponse{State: q.Get("state"), Code: q.Get("code"), Issuer: q.Get("iss"), Error: q.Get("error")}
		if err := r.pending.accept(req.Context(), response); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Connected to Jaz</title><p>Connected. You can close this tab and return to Jaz.</p></html>`)
	})}
	context.AfterFunc(ctx, func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	})
	go func() {
		_ = server.Serve(r.loopback)
	}()
}
