package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if s.STT == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("voice is not configured; set MISTRAL_API_KEY"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("audio file is required: %w", err))
		return
	}
	defer file.Close()
	text, err := s.STT.Transcribe(r.Context(), file, header.Filename)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

func (s *Server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if s.TTS == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("voice is not configured; set MISTRAL_API_KEY"))
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("text is required"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	stream, err := s.TTS.Speak(r.Context(), req.Text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", s.TTS.ContentType())
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	_, _ = io.Copy(flushWriter{w: w, flusher: flusher}, stream)
}

type flushWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if n > 0 {
		f.flusher.Flush()
	}
	return n, err
}
