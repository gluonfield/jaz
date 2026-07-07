package devices

import (
	"net"
	"net/url"
	"strings"

	"github.com/wins/jaz/backend/internal/serverconfig"
)

func connectionBaseURL(cfg serverconfig.Config, requestBase string, localIP func() (string, bool)) string {
	if strings.TrimSpace(cfg.PublicURL) != "" {
		return serverconfig.ClientBaseURL(cfg)
	}
	if !loopbackURL(requestBase) {
		return requestBase
	}
	if base := configuredNonLoopbackBaseURL(cfg); base != "" {
		return base
	}
	if !bindsBeyondLoopback(cfg.Addr) {
		return requestBase
	}
	ip, ok := localIP()
	if !ok {
		return requestBase
	}
	return replaceURLHost(requestBase, ip)
}

func configuredNonLoopbackBaseURL(cfg serverconfig.Config) string {
	base := serverconfig.ClientBaseURL(cfg)
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	if hostLoopbackOrUnspecified(u.Hostname()) {
		return ""
	}
	return base
}

func replaceURLHost(raw, host string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	return u.String()
}

func bindsBeyondLoopback(addr string) bool {
	host := strings.TrimSpace(addr)
	if host == "" || strings.HasPrefix(host, ":") {
		return true
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		if u, err := url.Parse(host); err == nil {
			host = u.Host
		}
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	return !hostLoopback(host)
}

func loopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && hostLoopback(u.Hostname())
}

func hostLoopbackOrUnspecified(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		return true
	}
	return hostLoopback(host)
}

func hostLoopback(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func firstLocalNetworkIP() (string, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	for _, addr := range addrs {
		ip := addrIP(addr)
		if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if ip.IsPrivate() {
			return ip.String(), true
		}
	}
	for _, addr := range addrs {
		ip := addrIP(addr)
		if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		return ip.String(), true
	}
	return "", false
}

func addrIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}
