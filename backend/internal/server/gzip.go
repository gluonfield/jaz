package server

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// withGzip compresses responses for clients that accept it. Transcript
// payloads are highly repetitive JSON, so this matters most when the server
// runs on a VM away from the client. The wrapper implements http.Flusher so
// SSE streams keep flushing per event through the compressor.
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// Forwards Flusher only. No handler hijacks connections today; one that does
// (websockets) must bypass or extend this wrapper, or the assertion fails.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	_ = w.gz.Flush()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
