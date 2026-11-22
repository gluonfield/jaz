package httpapi

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func RequestBaseURL(r *http.Request) string {
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	return proto + "://" + host
}

func TrustedLocalRequest(r *http.Request) bool {
	if !LoopbackRequest(r) {
		return false
	}
	host := (&url.URL{Host: r.Host}).Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "CF-Connecting-IP"} {
		if r.Header.Get(name) != "" {
			return false
		}
	}
	return true
}

func LoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func firstHeaderValue(value string) string {
	part, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(part)
}
