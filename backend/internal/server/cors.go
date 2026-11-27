package server

import (
	"net/http"

	"github.com/wins/jaz/backend/internal/httpapi"
)

func withCORS(next http.Handler, trusted ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" && !httpapi.AllowedOrigin(origin, trusted...) {
			http.Error(w, "origin is not allowed", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-Jaz-Client-Platform, If-Match, If-None-Match, X-Jaz-App-Ticket")
			w.Header().Set("Access-Control-Expose-Headers", "ETag")
			if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r.WithContext(httpapi.WithTrustedOrigins(r.Context(), trusted)))
	})
}
