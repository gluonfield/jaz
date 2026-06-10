package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxMusicChartFeedBytes = 1 << 20

func (s *Server) handleMusicChartFeed(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing url"))
		return
	}
	feedURL, err := url.Parse(raw)
	if err != nil || feedURL.Scheme != "https" || !allowedMusicChartHost(feedURL.Hostname()) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported chart feed url"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Errorf("chart feed returned %s", res.Status))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(res.Body, maxMusicChartFeedBytes))
}

func allowedMusicChartHost(host string) bool {
	switch strings.ToLower(host) {
	case "itunes.apple.com", "rss.applemarketingtools.com", "rss.marketingtools.apple.com":
		return true
	default:
		return false
	}
}
