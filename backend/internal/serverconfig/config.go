package serverconfig

import (
	"net/url"
	"strings"
)

const (
	JazToolsMCPPath       = "/mcp/jaztools"
	JazToolsMCPCompatPath = "/mcp/jaz"
	JazmemMCPPath         = "/mcp/jazmem"
	MCPProxyPath          = "/mcp/proxy"
)

type Config struct {
	Addr      string
	PublicURL string
	WebDir    string
}

type URLs struct {
	JazToolsMCP string
	JazmemMCP   string
	MCPProxy    string
}

func New(addr string, publicURL ...string) Config {
	cfg := Config{Addr: strings.TrimSpace(addr)}
	if len(publicURL) > 0 {
		cfg.PublicURL = strings.TrimSpace(publicURL[0])
	}
	return cfg
}

func NewURLs(config Config) URLs {
	return URLs{
		JazToolsMCP: localURL(config.Addr, JazToolsMCPPath),
		JazmemMCP:   localURL(config.Addr, JazmemMCPPath),
		MCPProxy:    localURL(config.Addr, MCPProxyPath),
	}
}

func DisplayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

func ClientURL(config Config, key string) string {
	base := ClientBaseURL(config)
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return base + "?key=" + url.QueryEscape(key)
	}
	q := u.Query()
	q.Set("key", key)
	u.RawQuery = q.Encode()
	return u.String()
}

func ClientBaseURL(config Config) string {
	base := strings.TrimSpace(config.PublicURL)
	if base == "" {
		base = config.Addr
	}
	u, err := url.Parse(DisplayAddr(base))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return DisplayAddr(base)
	}
	u.Path = ""
	u.RawPath = ""
	u.Fragment = ""
	u.RawQuery = ""
	return u.String()
}

func localURL(addr, path string) string {
	host := strings.TrimSpace(addr)
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	return "http://" + host + path
}
