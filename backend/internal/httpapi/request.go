package httpapi

import (
	"net"
	"net/http"
	"strings"
)

type RequestInfo struct {
	IP        string
	UserAgent string
}

func RequestInfoFrom(r *http.Request) RequestInfo {
	return RequestInfo{
		IP:        requestIP(r),
		UserAgent: strings.TrimSpace(r.UserAgent()),
	}
}

func requestIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		raw := strings.TrimSpace(r.Header.Get(header))
		if raw == "" {
			continue
		}
		host := strings.TrimSpace(strings.Split(raw, ",")[0])
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.TrimSpace(host)
}
