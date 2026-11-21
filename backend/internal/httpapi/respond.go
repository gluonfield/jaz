package httpapi

import (
	"encoding/json"
	"html"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, err error) {
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}

func WriteOAuthCallbackHTML(w http.ResponseWriter, status int, title, message string, autoClose bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	script := ""
	if autoClose {
		script = `<script>setTimeout(function(){window.close()},1200)</script>`
	}
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Jaz</title></head><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;color:#333;background:#fff"><main style="width:min(640px,calc(100vw - 48px));text-align:center"><h2 style="margin:0 0 10px;font-size:22px;font-weight:600">` + html.EscapeString(title) + `</h2><p style="margin:0;color:#666;font-size:14px;line-height:1.5">` + html.EscapeString(message) + `</p></main>` + script + `</body></html>`))
}
